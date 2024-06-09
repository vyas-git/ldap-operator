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

package controller

import (
	"context"
	"fmt"

	"gopkg.in/ldap.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	ldapv1alpha1 "github.com/vyas-git/ldap-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ldapServer     = "localhost"
	ldapPort       = 1389
	ldapBindDN     = "cn=admin,dc=example,dc=org"
	ldapBindPasswd = "admin"
	baseDN         = "dc=example,dc=org"
)

// LdapUserReconciler reconciles a LdapUser object
type LdapUserReconciler struct {
	client.Client
	Log logr.Logger

	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ldap.gopkg.blog,resources=ldapusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ldap.gopkg.blog,resources=ldapusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ldap.gopkg.blog,resources=ldapusers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LdapUser object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *LdapUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	log := r.Log.WithValues("ldapuser", req.NamespacedName)

	// Fetch the LDAPUser instance
	ldapUser := &ldapv1alpha1.LdapUser{}
	err := r.Get(ctx, req.NamespacedName, ldapUser)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get LDAPUser")
		return ctrl.Result{}, err
	}
	// Connect to LDAP
	l, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", ldapServer, ldapPort))
	if err != nil {
		log.Error(err, "Failed to connect to LDAP server")
		return ctrl.Result{}, err
	}
	defer l.Close()

	err = l.Bind(ldapBindDN, ldapBindPasswd)
	if err != nil {
		log.Error(err, "Failed to bind to LDAP server")
		return ctrl.Result{}, err
	}

	// Search for the user
	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(uid=%s)", ldapUser.Spec.Username),
		[]string{"dn", "cn", "uid"},
		nil,
	)
	sr, err := l.Search(searchRequest)
	if err != nil {
		log.Error(err, "Failed to search LDAP")
		return ctrl.Result{}, err
	}

	if len(sr.Entries) == 0 {
		log.Info("User not found in LDAP, deleting resources", "username", ldapUser.Spec.Username)
		// Delete resources associated with the user
		r.deleteUserResources(ctx, ldapUser)
		return ctrl.Result{}, nil
	}

	log.Info("User found in LDAP, creating/updating resources", "username", ldapUser.Spec.Username)
	// Create or update resources associated with the user
	r.createOrUpdateUserResources(ctx, ldapUser)

	return ctrl.Result{}, nil
}

func (r *LdapUserReconciler) createOrUpdateUserResources(ctx context.Context, ldapUser *ldapv1alpha1.LdapUser) error {
	// Create Namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ldapUser.Spec.Username,
		},
	}
	if err := r.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	// Create ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ldapUser.Spec.Username),
			Namespace: ldapUser.Spec.Username,
		},
		Data: map[string]string{
			"example.config": "value",
		},
	}
	if err := r.Create(ctx, cm); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	// Create Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-pod", ldapUser.Spec.Username),
			Namespace: "ldap-space",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sh", "-c", "echo Hello, Kubernetes! && sleep 3600"},
				},
			},
		},
	}
	if err := r.Create(ctx, pod); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (r *LdapUserReconciler) deleteUserResources(ctx context.Context, ldapUser *ldapv1alpha1.LdapUser) error {
	// Delete Namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ldapUser.Spec.Username,
		},
	}
	if err := r.Delete(ctx, ns); err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LdapUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ldapv1alpha1.LdapUser{}).
		Complete(r)
}
