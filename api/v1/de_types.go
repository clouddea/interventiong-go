/*
Copyright 2024.

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

// DESpec defines the desired state of DE
type DESpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of DE. Edit de_types.go to remove/update
	Components []DEComponent `json:"components,omitempty"` // 任务的组件
	AutoStart  string        `json:"autoStart,omitempty"`  // 自动启动仿真的端点
}

type DEComponent struct {
	Name  string            `json:"name"`    // service的名字
	URL   string            `json:"url"`     // 如何获取模型的可执行文件
	Binds map[string]string `json:"volumes"` // PV的名字，如果存在，则不会自动创建
}

// DEStatus defines the observed state of DE
type DEStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Status string `json:"status"` // 状态 "", pending, ready, failed 自动启动失败
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DE is the Schema for the des API
type DE struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DESpec   `json:"spec,omitempty"`
	Status DEStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DEList contains a list of DE
type DEList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DE `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DE{}, &DEList{})
}
