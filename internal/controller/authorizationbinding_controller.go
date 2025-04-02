/*
Copyright 2025.

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

package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"registry-auth/pkg/model"
	"registry-auth/pkg/utils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sort"
	"strings"
	"sync"
	"time"

	registryv1 "registry-auth/api/v1"
)

const (
	// BindingLabelKeyForTemplate
	// binding 会自动打上该label ， 用于标记spec.ref
	BindingLabelKeyForTemplate = "registry.xuyunjin.top/with-binding"
)

const (
	FinalizerOfBinding = "registry.xuyunjin.top/binding-finalizer"
)

func newAuthorizationBindingReconciler(client client.Client, scheme *runtime.Scheme) Controller {
	return &authorizationBindingReconciler{
		client:                    client,
		scheme:                    scheme,
		distributesTimeoutSeconds: 10,
	}
}

// AuthorizationBindingReconciler reconciles a AuthorizationBinding object
type authorizationBindingReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	// todo 分发超时
	distributesTimeoutSeconds int
}

//+kubebuilder:rbac:groups=registry.xuyunjin.top,resources=authorizationbindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=registry.xuyunjin.top,resources=authorizationbindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=registry.xuyunjin.top,resources=authorizationbindings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AuthorizationBinding object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *authorizationBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("authorizationBindingReconciler").WithValues("name", req.Name)
	logger.Info("Reconcile AuthorizationBinding")
	var binding registryv1.AuthorizationBinding
	if err := r.client.Get(ctx, req.NamespacedName, &binding); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var action func(context.Context, registryv1.AuthorizationBinding) error
	if binding.DeletionTimestamp != nil {
		action = r.onDelete
	} else {
		action = r.onPatch
	}
	// on delete
	if err := action(ctx, binding); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
}

func (r *authorizationBindingReconciler) onDelete(ctx context.Context, binding registryv1.AuthorizationBinding) error {
	mergeFrom := client.MergeFrom(binding.DeepCopy())
	// 获取 Template
	ctx, _ = context.WithTimeout(ctx, time.Duration(r.distributesTimeoutSeconds)*time.Second)

	if err := r.deleteDistribute(ctx, binding.Name, nil); err != nil {
		return err
	}
	// 查询secret
	var secretList v1.SecretList
	if err := r.client.List(ctx, &secretList, client.MatchingLabels{RegistryAuthLabelKeyManagedBy: binding.Name}); err != nil {
		return err
	}
	if len(secretList.Items) == 0 {
		// remove finalizer
		finalizers := utils.CollectionRemove[string](binding.Finalizers, FinalizerOfBinding)
		sort.Strings(finalizers)
		binding.Finalizers = finalizers
		// PATCH UPDATE
		if err := r.client.Patch(ctx, &binding, mergeFrom); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (r *authorizationBindingReconciler) onPatch(ctx context.Context, binding registryv1.AuthorizationBinding) error {
	// deal with finalizer and label
	mergeFrom := client.MergeFrom(binding.DeepCopy())

	// 判断finalizer
	if !utils.CollectionHas[string](binding.Finalizers, FinalizerOfBinding) {
		// for binding
		finalizer := utils.CollectionInsert[string](binding.Finalizers, FinalizerOfBinding)
		sort.Strings(finalizer)
		binding.Finalizers = finalizer
		return r.client.Patch(ctx, &binding, mergeFrom)
	}

	// 判断labels
	if !utils.MapHasKeyAndValue[string, string](binding.Labels, BindingLabelKeyForTemplate, binding.Spec.Ref) {
		binding.Labels = utils.MapPut[string, string](binding.Labels, BindingLabelKeyForTemplate, binding.Spec.Ref)
		return r.client.Patch(ctx, &binding, mergeFrom)
	}

	// 获取 Template
	var template registryv1.AuthorizationTemplate
	if err := r.client.Get(ctx, client.ObjectKey{Name: binding.Spec.Ref}, &template); err != nil {
		return err
	}

	// 处理分发
	_ = r.distributes(ctx, template, binding)
	// 查询secret
	var secretList v1.SecretList
	if err := r.client.List(ctx, &secretList, client.MatchingLabels{RegistryAuthLabelKeyManagedBy: binding.Name}); err != nil {
		return err
	}
	secretNamespaces := utils.CollectionMap[v1.Secret, registryv1.BindingDistributeStatus](secretList.Items, func(secret v1.Secret) registryv1.BindingDistributeStatus {
		// 记录实际绑定的ServiceAccount
		var serviceAccount []string
		if secret.Annotations != nil {
			value, exist := secret.Annotations[SecretSuccessBindServiceAccountAnnotationKey]
			if exist && strings.TrimSpace(value) != "" {
				_ = json.Unmarshal([]byte(value), &serviceAccount)
			}
		}

		return registryv1.BindingDistributeStatus{
			Namespace:       secret.Namespace,
			Name:            secret.Name,
			ServiceAccounts: serviceAccount,
		}
	})
	binding.Status.Distributes = secretNamespaces
	return r.client.Status().Patch(ctx, &binding, mergeFrom)

}

func (r *authorizationBindingReconciler) deleteDistribute(ctx context.Context, bindingName string, distributes []registryv1.BindingDistribute) error {
	// 删除不在这些模版中的元素
	var secretList v1.SecretList
	if err := r.client.List(ctx, &secretList, client.MatchingLabels{RegistryAuthLabelKeyManagedBy: bindingName}); err != nil {
		return err
	}
	if len(secretList.Items) != 0 {
		// 待删除的Secret = 所有已存在的 - binding 中分发的
		willDeleteSecret := utils.CollectionOfDiffSet[v1.Secret, registryv1.BindingDistribute](secretList.Items, distributes, func(secret v1.Secret, distribute registryv1.BindingDistribute) bool {
			return secret.Namespace == distribute.Namespace
		})

		if len(willDeleteSecret) != 0 {
			// 开启多线程删除
			wg := &sync.WaitGroup{}
			wg.Add(len(willDeleteSecret))

			for _, secret := range willDeleteSecret {
				secret := secret
				go func(ctx context.Context, obj v1.Secret, client client.Client, wg *sync.WaitGroup) {
					defer wg.Done()
					if err := client.Delete(ctx, &obj); err != nil {
						// todo print logger
					}
				}(ctx, secret, r.client, wg)
			}
			wg.Wait()
			return nil
		}
	}
	return nil
}

func (r *authorizationBindingReconciler) distributes(ctx context.Context, template registryv1.AuthorizationTemplate, binding registryv1.AuthorizationBinding) error {
	ctx, _ = context.WithTimeout(ctx, time.Duration(r.distributesTimeoutSeconds)*time.Second)
	if err := r.deleteDistribute(ctx, binding.Name, binding.Spec.Distributes); err != nil {
		return err
	}

	// 分发Secret
	var distributeFunction = func(ctx context.Context, template registryv1.AuthorizationTemplate, distribute registryv1.BindingDistribute, cli client.Client, wg *sync.WaitGroup) {
		defer wg.Done()
		// 生成template 模版
		secretTemplate, err := templateConvertSecret(template, distribute.Namespace, distribute.ServiceAccounts, binding.Name)
		if err != nil {
			// todo print logger
			return
		}
		// 查找集群中存在的Secret
		var clusterExistsSecret v1.Secret
		if err := cli.Get(ctx, client.ObjectKey{Name: secretTemplate.Name, Namespace: secretTemplate.Namespace}, &clusterExistsSecret); err != nil {
			if errors.IsNotFound(err) {
				if err := cli.Create(ctx, &secretTemplate); err != nil {
					// todo print logger
				}
				return
			}
			// todo print logger
			return
		}
		// merge
		mergeFrom := client.MergeFrom(clusterExistsSecret.DeepCopy())
		clusterExistsSecret.Labels = utils.MapMerge[string, string](clusterExistsSecret.Labels, secretTemplate.Labels)
		clusterExistsSecret.Annotations = utils.MapMerge[string, string](clusterExistsSecret.Annotations, secretTemplate.Annotations)
		clusterExistsSecret.Data = secretTemplate.Data
		if err := cli.Patch(ctx, &clusterExistsSecret, mergeFrom); err != nil {
			// todo print logger
			return
		}
		return

	}
	wg := &sync.WaitGroup{}
	wg.Add(len(binding.Spec.Distributes))
	for _, dis := range binding.Spec.Distributes {
		dis := dis
		go distributeFunction(ctx, template, dis, r.client, wg)
	}
	wg.Wait()
	return nil

}

func templateConvertSecret(template registryv1.AuthorizationTemplate, distributeNamespace string, serviceAccounts []string, bindingName string) (v1.Secret, error) {
	authFunc := func(username, password string) string {
		origin := username + ":" + password
		return base64.StdEncoding.EncodeToString([]byte(origin))
	}

	auth := make(map[string]model.DockerConfigAuth, len(template.Spec.Registries))
	for _, item := range template.Spec.Registries {
		auth[item.Server] = model.DockerConfigAuth{
			Username: item.Username,
			Password: item.Password,
			Email:    item.Email,
			Auth:     authFunc(item.Username, item.Password),
		}
	}
	dockerConfigJson := model.DockerConfigJson{Auths: auth}
	jsonBytes, err := json.Marshal(dockerConfigJson)
	if err != nil {
		return v1.Secret{}, err
	}
	apiVersion := template.APIVersion
	apiVersion = strings.ReplaceAll(apiVersion, ".", "-")
	apiVersion = strings.ReplaceAll(apiVersion, "/", "-")

	if len(serviceAccounts) == 0 {
		serviceAccounts = make([]string, 0)
	}
	saListJsonBytes, err := json.Marshal(serviceAccounts)
	if err != nil {
		return v1.Secret{}, err
	}

	return v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiVersion + "-" + template.Name,
			Namespace: distributeNamespace,
			Labels: map[string]string{
				RegistryAuthLabelKeyManagedBy: bindingName,
			},
			Annotations: map[string]string{
				SecretExpectBindServiceAccountAnnotationKey: string(saListJsonBytes),
			},
		},
		Data: map[string][]byte{
			".dockerconfigjson": jsonBytes,
		},
		Type: v1.SecretTypeDockerConfigJson,
	}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *authorizationBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&registryv1.AuthorizationBinding{}).
		Complete(r)
}
