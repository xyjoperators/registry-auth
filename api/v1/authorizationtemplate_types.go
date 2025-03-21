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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AuthorizationTemplateSpec defines the desired state of AuthorizationTemplate
type AuthorizationTemplateSpec struct {
	// 定义制品服务认证中应该包含哪些信息
	Registries []RegistryTemplate `json:"registries" yaml:"registries"`
}

type RegistryTemplate struct {
	// 服务地址 如 127.0.0.1:8443
	Server string `json:"server" yaml:"server"`
	// 用户名
	Username string `json:"username" yaml:"username"`
	// 密码
	Password string `json:"password" yaml:"password"`
	// 邮箱(可选)
	Email *string `json:"email,omitempty" yaml:"email,omitempty"`
}

// AuthorizationTemplateStatus defines the observed state of AuthorizationTemplate
type AuthorizationTemplateStatus struct {
	// 记录被哪些分发绑定
	Bindings []string `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	// 状态记录
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rat,scope=Cluster
// +kubebuilder:printcolumn:name="REGISTRIES",type=string,JSONPath=`.spec.registries[*].server`
// +kubebuilder:printcolumn:name="BINDINGS",type=string,JSONPath=`.status.bindings[*]`

// AuthorizationTemplate is the Schema for the authorizationtemplates API
type AuthorizationTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthorizationTemplateSpec   `json:"spec,omitempty"`
	Status AuthorizationTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AuthorizationTemplateList contains a list of AuthorizationTemplate
type AuthorizationTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthorizationTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthorizationTemplate{}, &AuthorizationTemplateList{})
}
