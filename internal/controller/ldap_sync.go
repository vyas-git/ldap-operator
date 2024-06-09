package controller

import (
	"context"
	"fmt"

	"gopkg.in/ldap.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ldapv1alpha1 "github.com/vyas-git/ldap-operator/api/v1alpha1"
)

func (r *LdapUserReconciler) SyncLdapUsers(client client.Client) error {
	// Connect to LDAP and retrieve user data
	ctx := context.Background()
	ldapUsers, err := r.retrieveLdapUsers()
	if err != nil {
		return err
	}

	// List existing LDAPUser resources in Kubernetes
	existingLDAPUsers := &ldapv1alpha1.LdapUserList{}
	if err := client.List(ctx, existingLDAPUsers); err != nil {
		return err
	}

	// Convert existing LDAPUser resources to a map for efficient lookup
	existingLDAPUserMap := make(map[string]*ldapv1alpha1.LdapUser)
	for i := range existingLDAPUsers.Items {
		existingLDAPUserMap[existingLDAPUsers.Items[i].Name] = &existingLDAPUsers.Items[i]
	}

	// Reconcile LDAPUser resources
	for username := range ldapUsers {
		// Check if LDAPUser resource already exists
		if _, exists := existingLDAPUserMap[username]; !exists {
			// LDAPUser resource does not exist, create it
			newLDAPUser := &ldapv1alpha1.LdapUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      username,
					Namespace: "ldap-space", // Specify the namespace here
				},
				Spec: ldapv1alpha1.LdapUserSpec{
					Username: username,
				},
			}
			if err := client.Create(ctx, newLDAPUser); err != nil {
				return err
			}
			// Optionally log the creation of the LDAPUser resource
			r.Log.Info("Created LDAPUser resource", "username", username)
		}
	}

	// Delete LDAPUser resources that no longer exist in LDAP
	for _, ldapUser := range existingLDAPUsers.Items {
		if _, exists := ldapUsers[ldapUser.Spec.Username]; !exists {
			if err := client.Delete(ctx, &ldapUser); err != nil {
				return err
			}
			// Optionally log the deletion of the LDAPUser resource
			r.Log.Info("Deleted LDAPUser resource", "username", ldapUser.Spec.Username)
		}
	}

	return nil
}

func (r *LdapUserReconciler) retrieveLdapUsers() (map[string]struct{}, error) {
	// Establish connection to LDAP server
	l, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", ldapServer, ldapPort))
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// Bind to LDAP server with admin credentials
	err = l.Bind(ldapBindDN, ldapBindPasswd)
	if err != nil {
		return nil, err
	}

	// Search LDAP directory for users
	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=inetOrgPerson)",
		[]string{"uid"},
		nil,
	)
	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	// Extract usernames from LDAP search results
	ldapUsers := make(map[string]struct{})
	for _, entry := range sr.Entries {
		ldapUsers[entry.GetAttributeValue("uid")] = struct{}{}
	}

	return ldapUsers, nil
}
