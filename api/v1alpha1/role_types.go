package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoleSpec defines the desired state of Role.
type RoleSpec struct {
	// The name of a preset defined in the configuration file of the operator.
	// The `namespaces`, `roleRules` and `clusterRoleRules` from the preset will
	// be appended to the `namespaces`, `roleRules` and `clusterRoleRules`
	// defined in the Role resource.
	Preset string `json:"preset,omitempty"`
	// A list of subjects (users, groups, or service accounts) that will be
	// granted the permissions defined in the `roleRules` and
	// `clusterRoleRules`.
	Subjects []rbacv1.Subject `json:"subjects,omitempty"`
	// A list of namespaces where Roles and RoleBindings will be created, with
	// the permissions defined in the `roleRules`.
	Namespaces []string `json:"namespaces,omitempty"`
	// A list of rules to be applied to Roles created in the namespaces defined
	// in the `namespaces`.
	RoleRules []rbacv1.PolicyRule `json:"roleRules,omitempty"`
	// A list of rules to be applied to the ClusterRole created.
	ClusterRoleRules []rbacv1.PolicyRule `json:"clusterRoleRules,omitempty"`
}

// RoleStatus defines the observed state of Role.
type RoleStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Role is the Schema for the roles API.
// +kubebuilder:printcolumn:name="Succeeded",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Indicates whether the Role has been successfully reconciled"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason for the current status"
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,description="Message with more information, regarding the current status"
// +kubebuilder:printcolumn:name="Last Transition",type=date,JSONPath=`.status.conditions[?(@.type=="Ready")].lastTransitionTime`,description="Time when the condition was updated the last time"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Time when the Role was created"
type Role struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoleSpec   `json:"spec,omitempty"`
	Status RoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RoleList contains a list of Role.
type RoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Role `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Role{}, &RoleList{})
}
