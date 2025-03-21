package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"
)

type Controller interface {
	SetupWithManager(mgr ctrl.Manager) error
}

func SetUpByManager(mgr ctrl.Manager) error {
	var reconciles = []Controller{
		newAuthorizationBindingReconciler(mgr.GetClient(), mgr.GetScheme()),
		newAuthorizationTemplateReconciler(mgr.GetClient(), mgr.GetScheme()),
		newSecretReconciler(mgr.GetClient(), mgr.GetScheme()),
	}
	for _, r := range reconciles {
		if err := r.SetupWithManager(mgr); err != nil {
			return err
		}
	}
	return nil

}
