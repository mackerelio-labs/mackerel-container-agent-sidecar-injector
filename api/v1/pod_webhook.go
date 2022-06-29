/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var ignoredNamespaces = []string{
	metav1.NamespaceSystem,
	metav1.NamespacePublic,
}

// log is for logging in this package.
var podlog = logf.Log.WithName("pod-resource")

const admissionServiceAccountDefaultAPITokenMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
const (
	admissionWebhookAnnotationInjectKey        = "agent-injector.contrib.mackerel.io/inject"
	admissionWebhookAnnotationStatusKey        = "agent-injector.contrib.mackerel.io/status"
	annotationRolesKey                         = "agent-injector.contrib.mackerel.io/roles"
	annotationMackerelAPISecretName            = "agent-injector.contrib.mackerel.io/mackerel_apikey.secret_name"
	annotationMackerelAgentConfigConfigMapName = "agent-injector.contrib.mackerel.io/mackere_agent_config.configmap_name"
	annotationEnvMackerelAgentConfig           = "agent-injector.contrib.mackerel.io/env.mackere_agent_config"
	annotationsBasePath                        = "/metadata/annotations"
	envMackerelAPIKey                          = "MACKEREL_APIKEY"
	mackerelAgentConfigVolumeName              = "mackerel-agent-config-volume"
	mackerelAgentConfigMountPath               = "/etc/mackerel-agent/mackerel-agent.conf"
)

// SetupWebhookWithManager is ...
func (r *PodWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		WithDefaulter(r).
		For(&corev1.Pod{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups=core,resources=pods,verbs=create;update,versions=v1,name=mutate.pod.mackerel.io,admissionReviewVersions=v1

// PodWebhook represents ...
type PodWebhook struct {
	AgentAPIKey             string
	AgentKubeletPort        int
	AgentKubeletInsecureTLS bool
}

var _ admission.CustomDefaulter = &PodWebhook{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *PodWebhook) Default(ctx context.Context, obj runtime.Object) error {
	podlog.Info("execute pod webhook")

	var pod *corev1.Pod
	var ok bool
	if pod, ok = obj.(*corev1.Pod); !ok {
		err := errors.New("invalid type")
		handleError(err)
		return err
	}

	if !mutationRequired(&pod.ObjectMeta) {
		podlog.Info("no mutate needed", "name", pod.Name)
		return nil
	}

	err := r.mutatePod(pod)
	if err != nil {
		handleError(err)
	}

	return nil
}

func mutationRequired(metadata *metav1.ObjectMeta) bool {
	for _, namespace := range ignoredNamespaces {
		if metadata.Namespace == namespace {
			return false
		}
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	status := annotations[admissionWebhookAnnotationStatusKey]

	var required bool
	if strings.ToLower(status) == "injected" {
		required = false
	} else {
		switch strings.ToLower(annotations[admissionWebhookAnnotationInjectKey]) {
		default:
			required = false
		case "true":
			required = true
		}
	}

	podlog.Info("mutated", "mutationRequired", required)

	return required
}

func (r *PodWebhook) mutatePod(pod *corev1.Pod) error {
	var configVolume *corev1.Volume
	if name, ok := pod.Annotations[annotationMackerelAgentConfigConfigMapName]; ok && name != "" {
		configVolume = createMackerelAgentConfigVolume(name)
		pod.Spec.Volumes = append(pod.Spec.Volumes, *configVolume)

		if path, ok := pod.Annotations[annotationEnvMackerelAgentConfig]; !ok || path == "" {
			pod.Annotations[annotationEnvMackerelAgentConfig] = mackerelAgentConfigMountPath
		}
	}

	container, err := r.generateInjectedContainer(pod)
	if err != nil {
		return err
	}

	if configVolume != nil {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      configVolume.Name,
			MountPath: mackerelAgentConfigMountPath,
			SubPath:   "mackerel-agent.conf",
		})
	}

	pod.Spec.Containers = append(pod.Spec.Containers, *container)
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[admissionWebhookAnnotationStatusKey] = "injected"

	return nil
}

func (r *PodWebhook) generateInjectedContainer(pod *corev1.Pod) (*corev1.Container, error) {
	env := []corev1.EnvVar{
		{
			Name:  "MACKEREL_CONTAINER_PLATFORM",
			Value: "kubernetes",
		},
		{
			Name: "MACKEREL_KUBERNETES_KUBELET_HOST",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.hostIP",
				},
			},
		},
		{
			Name: "MACKEREL_KUBERNETES_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: "MACKEREL_KUBERNETES_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name:  "MACKEREL_KUBERNETES_KUBELET_READ_ONLY_PORT",
			Value: "0",
		},
	}

	if name, ok := pod.Annotations[annotationMackerelAPISecretName]; ok && name != "" {
		env = append(env, corev1.EnvVar{
			Name: envMackerelAPIKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					Key: envMackerelAPIKey,
				},
			},
		})
	} else {
		if r.AgentAPIKey == "" {
			return nil, fmt.Errorf("%s is not specified", envMackerelAPIKey)
		}

		// set MACKEREL_APIKEY via args
		env = append(env, corev1.EnvVar{
			Name:  envMackerelAPIKey,
			Value: r.AgentAPIKey,
		})
	}

	if r.AgentKubeletPort != -1 {
		env = append(env, corev1.EnvVar{
			Name:  "MACKEREL_KUBERNETES_KUBELET_PORT",
			Value: fmt.Sprint(r.AgentKubeletPort),
		})
	}

	if r.AgentKubeletInsecureTLS {
		env = append(env, corev1.EnvVar{
			Name:  "MACKEREL_KUBERNETES_KUBELET_INSECURE_TLS",
			Value: "true",
		})
	}

	if roles, ok := pod.Annotations[annotationRolesKey]; ok {
		env = append(env, corev1.EnvVar{
			Name:  "MACKEREL_ROLES",
			Value: roles,
		})
	}

	if conf, ok := pod.Annotations[annotationEnvMackerelAgentConfig]; ok {
		env = append(env, corev1.EnvVar{
			Name:  "MACKEREL_AGENT_CONFIG",
			Value: conf,
		})
	}

	agentContainer := &corev1.Container{
		Name:            "mackerel-container-agent",
		Image:           "mackerel/mackerel-container-agent:plugins",
		ImagePullPolicy: corev1.PullAlways,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		Env: env,
	}

	// Find volumeMount injected by ServiceAccount Admission Plugin
	// https://github.com/kubernetes/kubernetes/blob/56b40066d5/plugin/pkg/admission/serviceaccount/admission.go#L473-502
containers:
	for _, c := range pod.Spec.Containers {
		for _, v := range c.VolumeMounts {
			podlog.Info("volumeMounted", "volume", v)

			if v.MountPath == admissionServiceAccountDefaultAPITokenMountPath {
				agentContainer.VolumeMounts = []corev1.VolumeMount{v}
				break containers
			}
		}
	}

	return agentContainer, nil
}

func createMackerelAgentConfigVolume(configMapName string) *corev1.Volume {
	return &corev1.Volume{
		Name: mackerelAgentConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}
}

func handleError(err error) {
	podlog.Error(err, "failed to inject")
}
