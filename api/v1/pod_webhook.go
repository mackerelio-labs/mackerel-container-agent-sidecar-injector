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
	admissionWebhookAnnotationInjectKey = "agent-injector.contrib.mackerel.io/inject"
	admissionWebhookAnnotationStatusKey = "agent-injector.contrib.mackerel.io/status"
	annotationRolesKey                  = "agent-injector.contrib.mackerel.io/roles"
	annotationsBasePath                 = "/metadata/annotations"
)

// SetupWebhookWithManager is ...
func (r *PodWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		WithDefaulter(r).
		For(&corev1.Pod{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups=core,resources=pods,verbs=create;update,versions=v1,name=mutate.pod.mackerel.io,admissionReviewVersions=v1

// PodWebhook represents ...
type PodWebhook struct {
	AgentAPIKey              string
	AgentKubeletPort         int
	AgentKubeletReadOnlyPort int
	AgentKubeletInsecureTLS  bool
}

var _ admission.CustomDefaulter = &PodWebhook{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *PodWebhook) Default(ctx context.Context, obj runtime.Object) error {
	podlog.Info("execute pod webhook")

	var pod *corev1.Pod
	var ok bool
	if pod, ok = obj.(*corev1.Pod); !ok {
		podlog.Info("cast failed.")
		return errors.New("invalid type")
	}
	if !mutationRequired(&pod.ObjectMeta) {
		return nil
	}

	r.mutatePod(pod)
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

func (r *PodWebhook) mutatePod(pod *corev1.Pod) {
	pod.Spec.Containers = append(pod.Spec.Containers, r.generateInjectedContainer(pod))
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[admissionWebhookAnnotationStatusKey] = "injected"
}

func (r *PodWebhook) generateInjectedContainer(pod *corev1.Pod) corev1.Container {
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
			Value: "-1",
		},
	}

	if r.AgentAPIKey != "" {
		env = append(env, corev1.EnvVar{
			Name:  "MACKEREL_APIKEY",
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

	if r.AgentKubeletReadOnlyPort != -1 {
		env = append(env, corev1.EnvVar{
			Name:  "MACKEREL_KUBERNETES_KUBELET_READ_ONLY_PORT",
			Value: fmt.Sprint(r.AgentKubeletReadOnlyPort),
		})
	}

	if roles, ok := pod.Annotations[annotationRolesKey]; ok {
		env = append(env, corev1.EnvVar{
			Name:  "MACKEREL_ROLES",
			Value: roles,
		})
	}

	agentContainer := corev1.Container{
		Name:            "mackerel-container-agent",
		Image:           "mackerel/mackerel-container-agent:latest",
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

	return agentContainer
}
