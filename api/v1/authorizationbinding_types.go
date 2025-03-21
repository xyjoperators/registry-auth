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

// AuthorizationBindingSpec defines the desired state of AuthorizationBinding
type AuthorizationBindingSpec struct {
	// 绑定哪个 AuthorizationTemplate
	Ref string `json:"ref" yaml:"ref"`

	// 将东西分发到哪里(如果为Empty 表示暂不分发)
	Distributes []BindingDistribute `json:"distributes,omitempty" yaml:"distributes,omitempty"`
}

type BindingDistribute struct {
	// 分发到哪个namespace
	Namespace string `json:"namespace" yaml:"namespace"`
	// 指定ServiceAccount 绑定
	ServiceAccounts []string `json:"serviceAccounts,omitempty" yaml:"serviceAccounts,omitempty"`
}

// AuthorizationBindingStatus defines the observed state of AuthorizationBinding
type AuthorizationBindingStatus struct {
	Distributes []BindingDistributeStatus `json:"distributes,omitempty" yaml:"distributes,omitempty"`
	// 状态记录
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type BindingDistributeStatus struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
	// 指定ServiceAccount 绑定
	ServiceAccounts []string `json:"serviceAccounts,omitempty" yaml:"serviceAccounts,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rab,scope=Cluster
// +kubebuilder:printcolumn:name="REF",type=string,JSONPath=`.spec.ref`
// +kubebuilder:printcolumn:name="DISNAMESPACE",type=string,JSONPath=`.spec.distributes[*].namespace`

// AuthorizationBinding is the Schema for the authorizationbindings API
type AuthorizationBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthorizationBindingSpec   `json:"spec,omitempty"`
	Status AuthorizationBindingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AuthorizationBindingList contains a list of AuthorizationBinding
type AuthorizationBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthorizationBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthorizationBinding{}, &AuthorizationBindingList{})
}
