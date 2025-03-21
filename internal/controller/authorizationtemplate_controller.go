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
	"k8s.io/apimachinery/pkg/api/errors"
	"registry-auth/pkg/utils"
	"sort"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	registryv1 "registry-auth/api/v1"
)

const (
	FinalizerOfTemplate = "registry.xuyunjin.top/template-finalizer"
)

var ()

func newAuthorizationTemplateReconciler(client client.Client, scheme *runtime.Scheme) Controller {
	return &authorizationTemplateReconciler{
		client:                          client,
		scheme:                          scheme,
		cascadingDeletion:               true,
		cascadingDeletionTimeoutSeconds: 5,
	}
}

// AuthorizationTemplateReconciler reconciles a AuthorizationTemplate object
type authorizationTemplateReconciler struct {
	client client.Client
	scheme *runtime.Scheme

	// todo 在外面透出这个参数 表示是否允许集联删除
	// 若开启改该参数 则在template 删除时 ，会集联删除binding
	cascadingDeletion bool
	// 删除超时时间
	cascadingDeletionTimeoutSeconds int
}

//+kubebuilder:rbac:groups=registry.xuyunjin.top,resources=authorizationtemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=registry.xuyunjin.top,resources=authorizationtemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=registry.xuyunjin.top,resources=authorizationtemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AuthorizationTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *authorizationTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("authorizationTemplateReconciler").WithValues("name", req.Name)
	logger.Info("Reconcile AuthorizationTemplate")
	var template registryv1.AuthorizationTemplate
	if err := r.client.Get(ctx, req.NamespacedName, &template); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var action func(context.Context, registryv1.AuthorizationTemplate) error
	if template.DeletionTimestamp != nil {
		action = r.onDelete
	} else {
		action = r.onPatch
	}

	// on delete
	if err := action(ctx, template); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
}

func (r *authorizationTemplateReconciler) onDelete(ctx context.Context, template registryv1.AuthorizationTemplate) error {
	mergeFrom := client.MergeFrom(template.DeepCopy())
	// 获取binding
	var binding registryv1.AuthorizationBindingList
	if err := r.client.List(ctx, &binding, client.MatchingLabels{BindingLabelKeyForTemplate: template.Name}); err != nil {
		return err
	}

	// 如果binding 都已清理完成 则清空Finalizer
	if len(binding.Items) == 0 {
		finalizers := utils.CollectionRemove[string](template.Finalizers, FinalizerOfTemplate)
		sort.Strings(finalizers)
		template.Finalizers = finalizers
		// PATCH UPDATE
		if err := r.client.Patch(ctx, &template, mergeFrom); err != nil {
			return err
		}
		return nil
	}
	// 是否允许集联删除 如果允许则删除这些 binding
	if r.cascadingDeletion {
		willDeleteList := utils.CollectionFilter[registryv1.AuthorizationBinding](binding.Items, func(binding registryv1.AuthorizationBinding) bool {
			// 保留 DeletionTimestamp == nil 的等待删除
			return binding.DeletionTimestamp == nil
		})

		if len(willDeleteList) != 0 {
			timeoutCtx, _ := context.WithTimeout(ctx, time.Duration(r.cascadingDeletionTimeoutSeconds)*time.Second)
			// 计数器等待
			wg := &sync.WaitGroup{}
			wg.Add(len(willDeleteList))

			// 循环开启协程删除
			for _, delObj := range willDeleteList {
				delObj := delObj
				go func(ctx context.Context, obj registryv1.AuthorizationBinding, client client.Client, wg *sync.WaitGroup) {
					defer wg.Done()
					if err := client.Delete(ctx, &obj); err != nil {
						// todo print logger
					}
				}(timeoutCtx, delObj, r.client, wg)
			}

			wg.Wait()
		}
	}
	return nil
}

func (r *authorizationTemplateReconciler) onPatch(ctx context.Context, template registryv1.AuthorizationTemplate) error {
	mergeFrom := client.MergeFrom(template.DeepCopy())
	// 判断finalizer
	if !utils.CollectionHas[string](template.Finalizers, FinalizerOfTemplate) {
		finalizer := utils.CollectionInsert[string](template.Finalizers, FinalizerOfTemplate)
		sort.Strings(finalizer)
		template.Finalizers = finalizer
		return r.client.Patch(ctx, &template, mergeFrom)
	}

	// 查看集群中的绑定了该Template 的 binding
	var binding registryv1.AuthorizationBindingList
	if err := r.client.List(ctx, &binding, client.MatchingLabels{BindingLabelKeyForTemplate: template.Name}); err != nil {
		return err
	}

	bindingNames := utils.CollectionMap[registryv1.AuthorizationBinding, string](binding.Items, func(binding registryv1.AuthorizationBinding) string {
		return binding.Name
	})
	sort.Strings(bindingNames)
	template.Status.Bindings = bindingNames
	return r.client.Status().Patch(ctx, &template, mergeFrom)
}

// SetupWithManager sets up the controller with the Manager.
func (r *authorizationTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&registryv1.AuthorizationTemplate{}).
		Complete(r)
}
