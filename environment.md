# Environment Variables

## Config

 - `OSLUC_LOG_LEVEL` (default: `INFO`) - Set logging level
 - `OSLUC_IDENTITY_PREFIX` (default: `notset`) - Set identity prefix to match only LDAP identities
 - `KUBECONFIG` - Optional path to kubeconfig file, defaults to in cluster credentials
 - `OSLUC_LDAP_SYNC_CONFIG_PATH` (default: `sync.yaml`) - Path to LDAPSyncConfig file
 - `OSLUC_LAST_LOGON_DAYS_AGO` (default: `0`) - Delete user also if it hasn't logged in the last n days. n must be more than 14 days, disabled by default
 - `OSLUC_CONFIRM` (default: `FALSE`) - Confirm removal of inactive or not found users, default false

