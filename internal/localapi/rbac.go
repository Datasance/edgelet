package localapi

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type rbacPermission struct {
	APIGroups    []string
	Resource     string
	Verb         string
	ResourceName string
}

var localAPIAuthorizationGroups = []string{
	"agent.iofog.org/v3",
	"agent.datasance.com/v3",
}

func mapRequestToPermission(r *http.Request) (rbacPermission, bool) {
	path := r.URL.Path
	method := strings.ToUpper(r.Method)
	verb := toRBACVerb(method)
	switch {
	case path == "/v3/system/status":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/status", Verb: verb}, true
	case path == "/v3/system/info":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/info", Verb: verb}, true
	case path == "/v3/system/version":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/version", Verb: verb}, true
	case path == "/v3/system/provision":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/provision", Verb: verb}, true
	case path == "/v3/system/reload":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/reload", Verb: verb}, true
	case path == "/v3/system/prune":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/prune", Verb: verb}, true
	case path == "/v3/system/config":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/config", Verb: verb}, true
	case path == "/v3/system/controller/cert":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/controller/cert", Verb: "update"}, true
	case path == "/v3/system/config/switch":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/config/switch", Verb: "update"}, true
	case path == "/v3/system/gps":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "system/gps", Verb: verb}, true
	case path == "/v3/images":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "images", Verb: verb}, true
	case path == "/v3/images:pull":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "images/pull", Verb: verb}, true
	case strings.HasPrefix(path, "/v3/images:pull/"):
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "images/pull/status", Verb: verb}, true
	case path == "/v3/images:load":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "images/load", Verb: verb}, true
	case path == "/v3/images:prune":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "images/prune", Verb: verb}, true
	case path == "/v3/images:remove":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "images/remove", Verb: verb}, true
	case path == "/v3/microservices/config":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "microservices/config/self", Verb: verb}, true
	case path == "/v3/microservices/control":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "microservices/control/self", Verb: verb}, true
	case path == "/v3/ms":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "microservices", Verb: verb}, true
	case strings.HasPrefix(path, "/v3/ms/"):
		if id, ok := microserviceResourceName(path); ok {
			return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "microservices", Verb: verb, ResourceName: id}, true
		}
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "microservices", Verb: verb}, true
	case strings.HasPrefix(path, "/v3/deploy/microservices/"):
		id := strings.TrimSpace(strings.TrimPrefix(path, "/v3/deploy/microservices/"))
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "deploy/microservices", Verb: verb, ResourceName: id}, true
	case strings.HasPrefix(path, "/v3/deploy/microservices:apply/"):
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "deploy/microservices/apply/status", Verb: verb}, true
	case strings.HasPrefix(path, "/v3/deploy/registries/"):
		id := strings.TrimSpace(strings.TrimPrefix(path, "/v3/deploy/registries/"))
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "deploy/registries", Verb: verb, ResourceName: id}, true
	case strings.HasPrefix(path, "/v3/deploy/microservices"):
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "deploy/microservices", Verb: verb}, true
	case strings.HasPrefix(path, "/v3/deploy/registries"):
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "deploy/registries", Verb: verb}, true
	case path == "/v3/auth/whoami":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "auth/whoami", Verb: verb}, true
	case path == "/v3/auth/tokens" || path == "/v3/auth/tokens/revoke":
		return rbacPermission{APIGroups: localAPIAuthorizationGroups, Resource: "auth/tokens", Verb: verb}, true
	default:
		return rbacPermission{}, false
	}
}

func microserviceResourceName(path string) (string, bool) {
	rest := strings.TrimPrefix(path, "/v3/ms/")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 {
		return "", false
	}
	id := strings.TrimSpace(parts[0])
	if id == "" {
		return "", false
	}
	return id, true
}

func isAuthorized(claims jwt.MapClaims, p rbacPermission) bool {
	tokenUse, _ := claims["tokenUse"].(string)
	sub, _ := claims["sub"].(string)
	if strings.TrimSpace(tokenUse) == "localapi" && (strings.HasPrefix(sub, "system:localadmin:") || sub == "system:localadmin:bootstrap") {
		return true
	}

	iofogRaw, ok := claims["iofog.org"].(map[string]interface{})
	if !ok {
		return false
	}
	rbacRaw, ok := iofogRaw["rbac"].(map[string]interface{})
	if !ok {
		return false
	}
	rulesRaw, ok := rbacRaw["rulesByGroup"].(map[string]interface{})
	if !ok {
		return false
	}
	return rulesMatchAnyGroup(rulesRaw, p.APIGroups, p.Resource, p.Verb, p.ResourceName)
}

func rulesMatchAnyGroup(groups map[string]interface{}, apiGroups []string, resource, verb, resourceName string) bool {
	for _, group := range apiGroups {
		if rulesMatch(groups, group, resource, verb, resourceName) {
			return true
		}
	}
	return false
}

func rulesMatch(groups map[string]interface{}, group, resource, verb, resourceName string) bool {
	rulesRaw, exists := groups[group]
	if !exists {
		rulesRaw, exists = groups["*"]
		if !exists {
			return false
		}
	}
	rulesSlice, ok := rulesRaw.([]interface{})
	if !ok {
		return false
	}
	for _, ruleRaw := range rulesSlice {
		rule, ok := ruleRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if !ruleListMatch(rule["resources"], resource) {
			continue
		}
		if !ruleVerbMatch(rule["verbs"], verb) {
			continue
		}
		if resourceName != "" {
			if rawNames, exists := rule["resourceNames"]; exists && !ruleListMatch(rawNames, resourceName) {
				continue
			}
		}
		return true
	}
	return false
}

func ruleVerbMatch(raw interface{}, expected string) bool {
	values, ok := raw.([]interface{})
	if !ok {
		return false
	}
	expected = canonicalVerb(expected)
	for _, value := range values {
		s, ok := value.(string)
		if !ok {
			continue
		}
		if s == "*" || canonicalVerb(s) == expected {
			return true
		}
	}
	return false
}

func ruleListMatch(raw interface{}, expected string) bool {
	if expected == "" {
		return true
	}
	values, ok := raw.([]interface{})
	if !ok {
		return false
	}
	for _, value := range values {
		s, ok := value.(string)
		if !ok {
			continue
		}
		if s == "*" || strings.EqualFold(s, expected) {
			return true
		}
	}
	return false
}

func toRBACVerb(method string) string {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		return "get"
	case http.MethodPost:
		return "create"
	case http.MethodPatch, http.MethodPut:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return canonicalVerb(strings.ToLower(method))
	}
}

func canonicalVerb(verb string) string {
	switch strings.ToLower(strings.TrimSpace(verb)) {
	case "patch", "put":
		return "update"
	case "post":
		return "create"
	case "remove":
		return "delete"
	default:
		return strings.ToLower(strings.TrimSpace(verb))
	}
}
