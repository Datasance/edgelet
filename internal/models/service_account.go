package models

import (
	"cmp"
	"slices"
	"strings"
)

type ServiceAccount struct {
	Name    string                `json:"name" yaml:"name"`
	RoleRef ServiceAccountRoleRef `json:"roleRef" yaml:"roleRef"`
	Rules   []ServiceAccountRule  `json:"rules" yaml:"rules"`
}

type ServiceAccountRoleRef struct {
	Kind string `json:"kind" yaml:"kind"`
	Name string `json:"name" yaml:"name"`
}

type ServiceAccountRule struct {
	APIGroups     []string `json:"apiGroups" yaml:"apiGroups"`
	Resources     []string `json:"resources" yaml:"resources"`
	Verbs         []string `json:"verbs" yaml:"verbs"`
	ResourceNames []string `json:"resourceNames,omitempty" yaml:"resourceNames,omitempty"`
}

type RBACRuleV1 struct {
	Resources     []string `json:"resources"`
	Verbs         []string `json:"verbs"`
	ResourceNames []string `json:"resourceNames,omitempty"`
}

type RBACEnvelopeV1 struct {
	Version      string                  `json:"version"`
	RulesByGroup map[string][]RBACRuleV1 `json:"rulesByGroup"`
}

func (sa *ServiceAccount) CanonicalRBACV1() RBACEnvelopeV1 {
	envelope := RBACEnvelopeV1{
		Version: "v1",
		RulesByGroup: map[string][]RBACRuleV1{
			"edgelet.iofog.org/v1": {},
		},
	}
	if sa == nil {
		return envelope
	}
	for _, rawRule := range sa.Rules {
		rule := rawRule.ToRBACRuleV1()
		rule.Verbs = CanonicalizeVerbs(rule.Verbs)
		for _, group := range rawRule.APIGroups {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			envelope.RulesByGroup[group] = append(envelope.RulesByGroup[group], rule)
		}
	}
	for group := range envelope.RulesByGroup {
		slices.SortFunc(envelope.RulesByGroup[group], func(a, b RBACRuleV1) int {
			return cmp.Compare(rbacRuleSortKey(a), rbacRuleSortKey(b))
		})
	}
	return envelope
}

func (r ServiceAccountRule) ToRBACRuleV1() RBACRuleV1 {
	rule := RBACRuleV1{
		Resources: append([]string{}, r.Resources...),
		Verbs:     append([]string{}, r.Verbs...),
	}
	if len(r.ResourceNames) > 0 {
		rule.ResourceNames = append([]string{}, r.ResourceNames...)
	}
	slices.Sort(rule.Resources)
	slices.Sort(rule.Verbs)
	slices.Sort(rule.ResourceNames)
	return rule
}

func CanonicalizeVerbs(verbs []string) []string {
	normalized := make([]string, 0, len(verbs))
	seen := make(map[string]struct{})
	for _, verb := range verbs {
		v := strings.ToLower(strings.TrimSpace(verb))
		switch v {
		case "put", "patch":
			v = "update"
		case "post":
			v = "create"
		case "remove":
			v = "delete"
		}
		if v == "" {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		normalized = append(normalized, v)
	}
	slices.Sort(normalized)
	return normalized
}

func rbacRuleSortKey(r RBACRuleV1) string {
	return strings.Join(r.Resources, ",") + "|" + strings.Join(r.Verbs, ",") + "|" + strings.Join(r.ResourceNames, ",")
}

func (r RBACRuleV1) ToMap() map[string]any {
	result := map[string]any{
		"resources": r.Resources,
		"verbs":     r.Verbs,
	}
	if len(r.ResourceNames) > 0 {
		result["resourceNames"] = r.ResourceNames
	}
	return result
}
