package v1

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPodWebHook(t *testing.T) {
	got := NewPodWebHook()
	want := &PodWebhook{
		IgnoreNamespaces: []string{
			metav1.NamespaceSystem,
			metav1.NamespacePublic,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("NewPodWebHook() = %#v, want %#v", got, want)
	}
}
