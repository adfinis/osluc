# OSLUC

**O**pen**S**hift **L**DAP **U**sers **C**leaner

## What is it

OSLUC does following:

* Gets a list of all OpenShift users (equivalent of `oc get users -o yaml`) using in-kubernetes or KUBECONFIG variable provided credentials
* Binds to LDAP service by using configuration sourced from LDAPSyncConfig compatible YAML file.
  * File needs to be mounted inside the container, path to it is configured using `OSLUC_LDAP_SYNC_CONFIG_PATH` environmental variable
* Traverses subset of users from the list
  * Checks if user contains idenitities list with **one and only** entry prefixed with LDAP provider name. Prefix value is read from `OSLUC_USER_IDENTITY_PREFIX` environmental variable.
  * Searches LDAP for the presence and status of the user
  * If user is deactivated or not found, removes User and Identity resource - when `OSLUC_CONFIRM` is set to `TRUE` otherwise, it is a dry run


## How to run

* Create a CronJob
  * use this image
  * mount ConfigMap containing LDAPSyncConfig
  * mount Secret containing LDAP bind credentials
  * use separate service account
  * give service account permissions to use verbs `delete,get,list` on `User.user.openshift.io/v1` and `Identity.user.openshift.io/v1`
* Set environmental variables as per description in `environment.md` file

## TODO

* better way to read LDAPSyncConfig
* more tests
