package v1

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPodWebHook(t *testing.T) {

	var ignoredNamespaces = []string{
		metav1.NamespaceSystem,
		metav1.NamespacePublic,
	}
	podWebHook := NewPodWebHook()

	if !reflect.DeepEqual(ignoredNamespaces, podWebHook.IgnoreNamespaces) {
		t.Errorf("NewPodWebHook() = %v, want %v", podWebHook.IgnoreNamespaces, ignoredNamespaces)
	}
}
