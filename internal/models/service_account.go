package models

import (
	"sort"
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
			"agent.datasance.com/v3": {},
			"agent.iofog.org/v3":     {},
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
		sort.Slice(envelope.RulesByGroup[group], func(i, j int) bool {
			left := envelope.RulesByGroup[group][i]
			right := envelope.RulesByGroup[group][j]
			return strings.Join(left.Resources, ",")+"|"+strings.Join(left.Verbs, ",")+"|"+strings.Join(left.ResourceNames, ",") <
				strings.Join(right.Resources, ",")+"|"+strings.Join(right.Verbs, ",")+"|"+strings.Join(right.ResourceNames, ",")
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
	sort.Strings(rule.Resources)
	sort.Strings(rule.Verbs)
	sort.Strings(rule.ResourceNames)
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
	sort.Strings(normalized)
	return normalized
}

func (r RBACRuleV1) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"resources": r.Resources,
		"verbs":     r.Verbs,
	}
	if len(r.ResourceNames) > 0 {
		result["resourceNames"] = r.ResourceNames
	}
	return result
}
