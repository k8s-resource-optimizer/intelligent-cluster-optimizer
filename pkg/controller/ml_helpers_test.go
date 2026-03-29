package controller

// Shared test helpers for the controller package ML tests.

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// fakeKube returns a minimal fake Kubernetes client suitable for unit tests.
func fakeKube() kubernetes.Interface {
	return fake.NewSimpleClientset()
}
