package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ricobergerdev1alpha1 "github.com/ricoberger/role-operator/api/v1alpha1"
)

var _ = Describe("Role Controller", func() {
	Context("when reconciling a resource", func() {
		const resourceName = "test-resource"
		const targetNamespace = "default"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{Name: resourceName}
		reconciler := &RoleReconciler{}

		subjects := []rbacv1.Subject{{
			Kind:     rbacv1.GroupKind,
			Name:     "team-a",
			APIGroup: rbacv1.GroupName,
		}}
		roleRules := []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		}}
		clusterRoleRules := []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"nodes"},
			Verbs:     []string{"get", "list", "watch"},
		}}

		BeforeEach(func() {
			reconciler = &RoleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		AfterEach(func() {
			resource := &ricobergerdev1alpha1.Role{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// envtest has no garbage collector, therefore we clean the managed
			// objects up explicitly.
			_ = k8sClient.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: resourceName}})
			_ = k8sClient.Delete(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: resourceName}})
			_ = k8sClient.Delete(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: targetNamespace}})
			_ = k8sClient.Delete(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: targetNamespace}})
		})

		createRole := func(spec ricobergerdev1alpha1.RoleSpec) {
			resource := &ricobergerdev1alpha1.Role{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName},
				Spec:       spec,
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		}

		reconcileOnce := func() (reconcile.Result, error) {
			return reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		}

		It("should create Role, RoleBinding, ClusterRole and ClusterRoleBinding", func() {
			createRole(ricobergerdev1alpha1.RoleSpec{
				Subjects:         subjects,
				Namespaces:       []string{targetNamespace},
				RoleRules:        roleRules,
				ClusterRoleRules: clusterRoleRules,
			})

			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())

			By("creating the ClusterRole with the given rules")
			clusterRole := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, clusterRole)).To(Succeed())
			Expect(clusterRole.Rules).To(Equal(clusterRoleRules))
			Expect(clusterRole.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "role-operator"))
			Expect(clusterRole.Labels).To(HaveKeyWithValue("role-operator.ricoberger.de/role", resourceName))
			Expect(clusterRole.OwnerReferences).To(HaveLen(1))

			By("creating the ClusterRoleBinding with the given subjects")
			clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, clusterRoleBinding)).To(Succeed())
			Expect(clusterRoleBinding.Subjects).To(Equal(subjects))
			Expect(clusterRoleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(clusterRoleBinding.RoleRef.Name).To(Equal(resourceName))

			By("creating the Role in the target namespace")
			role := &rbacv1.Role{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNamespace}, role)).To(Succeed())
			Expect(role.Rules).To(Equal(roleRules))

			By("creating the RoleBinding in the target namespace")
			roleBinding := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNamespace}, roleBinding)).To(Succeed())
			Expect(roleBinding.Subjects).To(Equal(subjects))
			Expect(roleBinding.RoleRef.Kind).To(Equal("Role"))

			By("setting the Ready condition to True")
			updated := &ricobergerdev1alpha1.Role{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(updated.Status.Conditions, "Ready")).To(BeTrue())
		})

		It("should not create any objects when subjects are empty and mark the Role not ready", func() {
			createRole(ricobergerdev1alpha1.RoleSpec{
				Namespaces:       []string{targetNamespace},
				RoleRules:        roleRules,
				ClusterRoleRules: clusterRoleRules,
			})

			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())

			By("not creating the ClusterRole or the Role")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, &rbacv1.ClusterRole{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNamespace}, &rbacv1.Role{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			By("not creating the bindings")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, &rbacv1.ClusterRoleBinding{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNamespace}, &rbacv1.RoleBinding{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			By("setting the Ready condition to False with reason NoSubjects")
			updated := &ricobergerdev1alpha1.Role{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("NoSubjects"))
		})

		It("should not create cluster-scoped objects when clusterRoleRules are empty", func() {
			createRole(ricobergerdev1alpha1.RoleSpec{
				Subjects:   subjects,
				Namespaces: []string{targetNamespace},
				RoleRules:  roleRules,
			})

			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, &rbacv1.ClusterRole{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, &rbacv1.ClusterRoleBinding{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should skip namespaces that do not exist", func() {
			createRole(ricobergerdev1alpha1.RoleSpec{
				Subjects:   subjects,
				Namespaces: []string{targetNamespace, "does-not-exist"},
				RoleRules:  roleRules,
			})

			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNamespace}, &rbacv1.Role{})).To(Succeed())
			err = k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: "does-not-exist"}, &rbacv1.Role{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should prune Roles when a namespace is removed from the spec", func() {
			createRole(ricobergerdev1alpha1.RoleSpec{
				Subjects:   subjects,
				Namespaces: []string{targetNamespace},
				RoleRules:  roleRules,
			})

			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNamespace}, &rbacv1.Role{})).To(Succeed())

			By("removing the namespace from the spec")
			resource := &ricobergerdev1alpha1.Role{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			resource.Spec.Namespaces = []string{}
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			_, err = reconcileOnce()
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: targetNamespace}, &rbacv1.Role{})
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("should return not found without error when the Role is deleted", func() {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "missing"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
})
