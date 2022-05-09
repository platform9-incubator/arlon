/*
Copyright 2021.

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

// CallHomeConfigSpec defines the desired state of CallHomeConfig
type CallHomeConfigSpec struct {
	ServiceAccountName      string `json:"serviceAccountName"`      //
	KubeconfigSecretName    string `json:"kubeconfigSecretName"`    //
	KubeconfigSecretKeyName string `json:"kubeconfigSecretKeyName"` //
	TargetNamespace         string `json:"targetNamespace"`         //
	TargetSecretName        string `json:"targetSecretName"`        //
	TargetSecretKeyName     string `json:"targetSecretKeyName"`     //
	ManagementClusterUrl    string `json:"managementClusterUrl"`    //
}

// CallHomeConfigStatus defines the observed state of CallHomeConfig
type CallHomeConfigStatus struct {
	State   string `json:"state"`   // "retrying", "error", or "complete"
	Message string `json:"message"` // for "retrying" status
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CallHomeConfig is the Schema for the callhomeconfigs API
type CallHomeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CallHomeConfigSpec   `json:"spec,omitempty"`
	Status CallHomeConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CallHomeConfigList contains a list of CallHomeConfig
type CallHomeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CallHomeConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CallHomeConfig{}, &CallHomeConfigList{})
}
