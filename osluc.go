package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env"
	"github.com/go-ldap/ldap/v3"
	userv1 "github.com/openshift/client-go/user/clientset/versioned/typed/user/v1"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

//go:generate envdoc --output environment.md
type Config struct {
	// Set logging level
	LogLevel string `env:"OSLUC_LOG_LEVEL" envDefault:"INFO"`
	// Set identity prefix to match only LDAP identities
	IdentityPrefix string `env:"OSLUC_IDENTITY_PREFIX" envDefault:"notset"`
	// Optional path to kubeconfig file, defaults to in cluster credentials
	Kubeconfig string `env:"KUBECONFIG" envDefault:""`
	// Path to LDAPSyncConfig file
	LDAPSyncConfigPath string `env:"OSLUC_LDAP_SYNC_CONFIG_PATH" envDefault:"sync.yaml"`
	// Confirm removal of inactive or not found users, default false
	Confirm bool `env:"OSLUC_CONFIRM" envDefault:"FALSE"`
}

func main() {
	// logging and configuration read setup
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		slog.Error("Parsing environment variables", "error", err)
	}

	level := slog.LevelInfo
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		slog.Warn("Couldn't parse OSLUC_LOG_LEVEL, defaulting to INFO")
	}
	slog.Info("Logging setup", "level", level)

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	slog.Info("OSLUC confirm flag", "confirm", cfg.Confirm)

	// create kubernetes client
	cl, err := createClient(cfg.Kubeconfig)
	if err != nil {
		slog.Error("Unable to create Kubernetes client", "error", err)
		os.Exit(1)
	}

	// get all users
	users, err := cl.Users().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		slog.Error("Unable to list all users", "error", err)
		os.Exit(1)
	}
	slog.Info("Found users", "total", len(users.Items))

	// read LDAPSyncConfig
	var ldapSyncCfg map[string]interface{}
	yamlFile, err := os.ReadFile(cfg.LDAPSyncConfigPath)
	if err != nil {
		slog.Error("Unable to read LDAPSyncConfig file", "error", err)
		os.Exit(1)
	}
	err = yaml.Unmarshal(yamlFile, &ldapSyncCfg)
	if err != nil {
		slog.Error("Unable to parse LDAPSyncConfig file", "error", err)
		os.Exit(1)
	}

	// connect to LDAP
	ldap := bindLDAP(ldapSyncCfg)
	defer ldap.Close()

	// Do the check and delete
	for _, user := range users.Items {
		// only if there is one identity
		if len(user.Identities) == 1 {
			for _, id := range user.Identities {
				if strings.HasPrefix(id, cfg.IdentityPrefix) {
					slog.Debug("Found User with correct prefix, searching in LDAP", "user", user.Name)
					if searchUser(ldap, ldapSyncCfg, user.Name) {
						slog.Info("Remove User and identity", "name", user, "confirm", cfg.Confirm)
						if cfg.Confirm {
							// delete Identity
							slog.Debug("Deleting identity", "name", id)
							err_id := cl.Identities().Delete(context.TODO(), id, metav1.DeleteOptions{})
							if err_id != nil {
								slog.Error("Unable to delete identity", "error", err_id)
							} else {
								slog.Info("Successfully deleted identity", "name", id)
								// delete User
								slog.Debug("Deleting user", "name", user.Name)
								err_user := cl.Users().Delete(context.TODO(), user.Name, metav1.DeleteOptions{})
								if err_user != nil {
									slog.Error("Unable to delete user", "error", err_user)
								} else {
									slog.Info("Successfully deleted user", "name", user.Name)
								}
							}

						}
					}
				} else {
					slog.Debug("User identity prefix is wrong, skipping",
						"user", user.Name,
						"identity prefix", strings.Split(id, ":")[0],
						"expected prefix", cfg.IdentityPrefix)
				}
			}
		} else {
			slog.Debug("Skipping user due to identity count mismatch", "user", user.Name, "expected", "1", "got", len(user.Identities))
		}
	}
}

func bindLDAP(cfg map[string]interface{}) *ldap.Conn {

	cfgBindPassword := cfg["bindPassword"].(map[string]interface{})
	cfgBindPasswordFile := cfgBindPassword["file"].(string)

	bindPassword, err := os.ReadFile(filepath.Clean(cfgBindPasswordFile))
	if err != nil {
		slog.Error("Unable to read password file", "error", err)
		os.Exit(1)
	}

	// Connect to LDAP server using DialURL
	l, err := ldap.DialURL(fmt.Sprint(cfg["url"]))
	if err != nil {
		slog.Error("Failed to connect to LDAP", "error", err)
		os.Exit(1)
	}

	// Bind with a service account
	err = l.Bind(fmt.Sprint(cfg["bindDN"]), string(bindPassword))
	if err != nil {
		slog.Error("Failed to bind to LDAP", "error", err)
		os.Exit(1)
	}

	slog.Debug("Successfully bound to LDAP")

	return l
}

// returns true if user not found or inactive
func searchUser(l *ldap.Conn, cfg map[string]interface{}, username string) bool {
	aad := cfg["augmentedActiveDirectory"].(map[string]interface{})
	aadUq := aad["usersQuery"].(map[string]interface{})
	baseDN := aadUq["baseDN"].(string)
	filter := aadUq["filter"].(string)

	// handle derefAliases
	derefAliases := ldap.NeverDerefAliases
	if aadUq["derefAliases"].(string) == "always" {
		derefAliases = ldap.DerefAlways
	}

	// handle userNameAttributes
	aadUserNames := aad["userNameAttributes"].([]any)
	usernameFilter := "|"
	for _, filterUser := range aadUserNames {
		usernameFilter = fmt.Sprintf("%s(%s=%s)", usernameFilter, filterUser, username)
	}

	// put filter together
	searchFilter := fmt.Sprintf("(%s)", usernameFilter)
	if filter != "" {
		searchFilter = fmt.Sprintf("(&%s%s)", filter, searchFilter)
	}
	slog.Debug("Using LDAP filter", "filter", searchFilter)

	// Search for the user
	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		derefAliases,
		0,            // Size limit, 0 for no limit
		0,            // Time limit, 0 for no limit
		false,        // TypesOnly - don't return attribute values, only attribute types
		searchFilter, // Search filter
		[]string{"dn", "cn", "lastLogon", "accountExpires", "sAMAccountName", "lastLogonTimestamp"}, // Attributes to retrieve //
		nil,
	)

	// Perform the search
	result, err := l.Search(searchRequest)
	if err != nil {
		slog.Error("LDAP search failed", "error", err)
		os.Exit(1)
	}

	if len(result.Entries) == 0 {
		slog.Debug("User not found in LDAP, can be removed", "filter", searchFilter)
		return true
	} else {
		slog.Debug("User found in LDAP, checking user attributes", "filter", searchFilter)
		for _, entry := range result.Entries {

			lastLogon, err := FileTimeToGoTime(entry.GetAttributeValue("lastlogon"))
			if err != nil {
				slog.Error("Cannot convert FileTime for lastLogon attribute", "error", err)
			}

			lastLogonTS, err := FileTimeToGoTime(entry.GetAttributeValue("lastlogontimestamp"))
			if err != nil {
				slog.Error("Cannot convert FileTime for lastLogonTimestamp attribute", "error", err)
			}

			accountExpires, err := FileTimeToGoTime(entry.GetAttributeValue("accountexpires"))
			if err != nil {
				slog.Error("Cannot convert FileTime for accountExpires attribute", "error", err)
			}

			now := time.Now()
			slog.Debug("Attributes",
				"DN", entry.DN,
				"CN", entry.GetAttributeValue("cn"),
				"sAMAccountName", entry.GetAttributeValue("samaccountname"),
				"lastLogon", lastLogon,
				"lastLogonAgo", fmt.Sprintf("%v days", int(now.Sub(lastLogon).Hours()/24)),
				"lastLogonTimestamp", lastLogonTS,
				"lastLogonTimestampAgo", fmt.Sprintf("%v days", int(now.Sub(lastLogonTS).Hours()/24)),
				"accountExpires", accountExpires,
				"accountExpiresAgo", fmt.Sprintf("%v days", int(now.Sub(lastLogonTS).Hours()/24)),
			)
		}
	}

	return false
}

func createClient(kubeconfigPath string) (userv1.UserV1Interface, error) {
	var kubeconfig *rest.Config

	if kubeconfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("unable to load kubeconfig from %s: %v", kubeconfigPath, err)
		}
		kubeconfig = config
	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to load in-cluster config: %v", err)
		}
		kubeconfig = config
	}

	userV1Client, err := userv1.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create a client: %v", err)
	}

	return userV1Client, nil
}

func FileTimeToGoTime(fileTimeStr string) (time.Time, error) {
	// Convert the file time string to an integer
	fileTimeInt, err := strconv.ParseInt(fileTimeStr, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid fileTime: %v", err)
	}

	// Constants
	const windowsEpochOffset = 11644473600 // Difference in seconds between 1601-01-01 and 1970-01-01
	const hundredNanosecondsPerSecond = 10000000

	// Convert Windows FileTime to UNIX timestamp
	unixTimestamp := (fileTimeInt / hundredNanosecondsPerSecond) - windowsEpochOffset

	// Convert UNIX timestamp to Go time.Time in UTC
	goTime := time.Unix(unixTimestamp, 0).UTC()

	return goTime, nil
}
