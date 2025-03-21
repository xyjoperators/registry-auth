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
	"encoding/json"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"registry-auth/pkg/utils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// SecretExpectBindServiceAccountAnnotationKey
	// 记录secret 期望绑定的SA
	SecretExpectBindServiceAccountAnnotationKey = "registry.xuyunjin.top/expect-bind-sa"

	// SecretSuccessBindServiceAccountAnnotationKey
	// 记录secret 实际绑定的SA
	SecretSuccessBindServiceAccountAnnotationKey = "registry.xuyunjin.top/bind-sa"
)
const (
	// RegistryAuthLabelKeyManagedBy
	// 匹配的Secret 会打上该label 表示被纳管理
	RegistryAuthLabelKeyManagedBy = "registry.xuyunjin.top/auth-managed-by"
)

const (
	FinalizerOfManagedSecret = "registry.xuyunjin.top/secret-finalizer"
)

func newSecretReconciler(client client.Client, scheme *runtime.Scheme) Controller {
	return &secretReconciler{
		client:                    client,
		scheme:                    scheme,
		distributesTimeoutSeconds: 10,
	}
}

// SecretReconciler reconciles a Secret object
type secretReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	// todo 分发超时
	distributesTimeoutSeconds int
}

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Secret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *secretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("secretReconciler").WithValues("name", req.Name)
	logger.Info("Reconcile managed docker config json Secret")

	var secret v1.Secret
	if err := r.client.Get(ctx, req.NamespacedName, &secret); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var action func(context.Context, v1.Secret) error
	if secret.DeletionTimestamp != nil {
		action = r.onDelete
	} else {
		action = r.onPatch
	}

	// on delete
	if err := action(ctx, secret); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 1 * time.Second}, nil

}
func (r *secretReconciler) onDelete(ctx context.Context, secret v1.Secret) error {
	mergeFrom := client.MergeFrom(secret.DeepCopy())

	// 获取 Template
	ctx, _ = context.WithTimeout(ctx, time.Duration(r.distributesTimeoutSeconds)*time.Second)
	if err := r.deleteDistribute(ctx, secret.Namespace, secret.Name, nil); err != nil {
		return err
	}

	// 获取namespace 下的所有SA 如果不包含当前secret 则可以移除finalizer
	var serviceAccountList v1.ServiceAccountList
	if err := r.client.List(ctx, &serviceAccountList, client.InNamespace(secret.Namespace)); err != nil {
		return err
	}
	matchList := utils.CollectionFilter[v1.ServiceAccount](serviceAccountList.Items, func(account v1.ServiceAccount) bool {
		return utils.CollectionHas[v1.LocalObjectReference](account.ImagePullSecrets, v1.LocalObjectReference{
			Name: secret.Name,
		})
	})

	if len(matchList) == 0 {
		// remove finalizer
		finalizers := utils.CollectionRemove[string](secret.Finalizers, FinalizerOfManagedSecret)
		sort.Strings(finalizers)
		secret.Finalizers = finalizers
		// PATCH UPDATE
		if err := r.client.Patch(ctx, &secret, mergeFrom); err != nil {
			return err
		}
		return nil
	}
	return nil

}

func (r *secretReconciler) onPatch(ctx context.Context, secret v1.Secret) error {
	// deal with finalizer and label
	mergeFrom := client.MergeFrom(secret.DeepCopy())
	// 判断finalizer
	if !utils.CollectionHas[string](secret.Finalizers, FinalizerOfManagedSecret) {
		// for binding
		finalizer := utils.CollectionInsert[string](secret.Finalizers, FinalizerOfManagedSecret)
		sort.Strings(finalizer)
		secret.Finalizers = finalizer
		return r.client.Patch(ctx, &secret, mergeFrom)
	}
	var expectServiceAccountName []string

	if secret.Annotations != nil {
		value, exist := secret.Annotations[SecretExpectBindServiceAccountAnnotationKey]
		if exist && strings.TrimSpace(value) != "" {
			if err := json.Unmarshal([]byte(value), &expectServiceAccountName); err != nil {
				return err
			}
		}
	}
	// 处理分发
	_ = r.distributes(ctx, secret, expectServiceAccountName)

	var serviceAccountList v1.ServiceAccountList
	if err := r.client.List(ctx, &serviceAccountList, client.InNamespace(secret.Namespace)); err != nil {
		return err
	}
	matchList := utils.CollectionFilter[v1.ServiceAccount](serviceAccountList.Items, func(account v1.ServiceAccount) bool {
		return utils.CollectionHas[v1.LocalObjectReference](account.ImagePullSecrets, v1.LocalObjectReference{
			Name: secret.Name,
		})
	})
	successServiceAccountNames := utils.CollectionMap[v1.ServiceAccount, string](matchList, func(account v1.ServiceAccount) string {
		return account.Name
	})
	sort.Strings(successServiceAccountNames)

	bytes, _ := json.Marshal(successServiceAccountNames)
	secret.Annotations = utils.MapPut[string, string](secret.Annotations, SecretSuccessBindServiceAccountAnnotationKey, string(bytes))
	return r.client.Patch(ctx, &secret, mergeFrom)

}
func (r *secretReconciler) distributes(ctx context.Context, secret v1.Secret, expectServiceAccounts []string) error {
	ctx, _ = context.WithTimeout(ctx, time.Duration(r.distributesTimeoutSeconds)*time.Second)
	if err := r.deleteDistribute(ctx, secret.Namespace, secret.Name, expectServiceAccounts); err != nil {
		return err
	}
	var distributeFunction = func(ctx context.Context, namespace, secretName, serviceAccountName string, cli client.Client, wg *sync.WaitGroup) {
		defer wg.Done()
		// 获取serviceAccount
		var serviceAccount v1.ServiceAccount
		if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: serviceAccountName}, &serviceAccount); err != nil {
			// todo print logger
			return
		}
		mergeFrom := client.MergeFrom(serviceAccount.DeepCopy())
		// service account 的 image pull secret 中如果没有secret name 则添加一个
		if !utils.CollectionHas[v1.LocalObjectReference](serviceAccount.ImagePullSecrets, v1.LocalObjectReference{
			Name: secretName,
		}) {
			serviceAccount.ImagePullSecrets = utils.CollectionInsert[v1.LocalObjectReference](serviceAccount.ImagePullSecrets, v1.LocalObjectReference{
				Name: secretName,
			})
			if err := cli.Patch(ctx, &serviceAccount, mergeFrom); err != nil {
				// todo print logger
				return
			}
			return
		}
	}
	wg := &sync.WaitGroup{}
	wg.Add(len(expectServiceAccounts))
	for _, expectServiceAccount := range expectServiceAccounts {
		expectServiceAccount := expectServiceAccount
		go distributeFunction(ctx, secret.Namespace, secret.Name, expectServiceAccount, r.client, wg)
	}
	wg.Wait()
	return nil

}

func (r *secretReconciler) deleteDistribute(ctx context.Context, namespace string, secretName string, serviceAccountNames []string) error {
	var serviceAccountList v1.ServiceAccountList
	if err := r.client.List(ctx, &serviceAccountList, client.InNamespace(namespace)); err != nil {
		return err
	}
	if len(serviceAccountList.Items) == 0 {
		return nil
	}
	// 先过滤哪些service account 可能需要移除 secret
	willRemoveServiceAccount := utils.CollectionOfDiffSet[v1.ServiceAccount, string](serviceAccountList.Items, serviceAccountNames, func(serviceAccount v1.ServiceAccount, serviceAccountName string) bool {
		return serviceAccount.Name == serviceAccountName
	})
	// 再过滤出包含该imagePullSecret serviceAccount
	willRemoveServiceAccount = utils.CollectionFilter[v1.ServiceAccount](willRemoveServiceAccount, func(account v1.ServiceAccount) bool {
		return utils.CollectionHas[v1.LocalObjectReference](account.ImagePullSecrets, v1.LocalObjectReference{
			Name: secretName,
		})
	})

	if len(willRemoveServiceAccount) != 0 {
		// 开启多线程移除
		wg := &sync.WaitGroup{}
		wg.Add(len(willRemoveServiceAccount))

		for _, serviceAccount := range willRemoveServiceAccount {
			serviceAccount := serviceAccount
			go func(ctx context.Context, obj v1.ServiceAccount, secretName string, cli client.Client, wg *sync.WaitGroup) {
				defer wg.Done()
				mergeFrom := client.MergeFrom(obj.DeepCopy())
				obj.ImagePullSecrets = utils.CollectionRemove(obj.ImagePullSecrets, v1.LocalObjectReference{Name: secretName})
				if err := cli.Patch(ctx, &obj, mergeFrom); err != nil {
					// todo print logger
				}
			}(ctx, serviceAccount, secretName, r.client, wg)
		}
		wg.Wait()
		return nil
	}
	return nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *secretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&v1.Secret{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool {
				return onlyDealWithOperatorSecret(event.Object)
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				return onlyDealWithOperatorSecret(updateEvent.ObjectNew)
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				return onlyDealWithOperatorSecret(deleteEvent.Object)
			},
		}).
		Complete(r)
}

// 只处理Operator 管理的Secret
func onlyDealWithOperatorSecret(object client.Object) bool {
	var secret = object.(*v1.Secret)
	if secret.Type != v1.SecretTypeDockerConfigJson {
		return false
	}
	if !utils.MapHasKey[string, string](secret.Labels, RegistryAuthLabelKeyManagedBy) {
		return false
	}
	return true

}
