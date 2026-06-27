package fieldagent

import (
	"encoding/json"
	"slices"
	"strings"
)

// ListActiveControllerSessionIDs returns sorted controller attachment session ids for one microservice.
func (esm *ExecSessionManager) ListActiveControllerSessionIDs(msUUID string) []string {
	if esm == nil {
		return nil
	}
	msUUID = strings.TrimSpace(msUUID)
	if msUUID == "" {
		return nil
	}

	esm.mu.RLock()
	defer esm.mu.RUnlock()

	ids := make([]string, 0, len(esm.activeSessions))
	for sessionID, info := range esm.activeSessions {
		if info == nil || info.Session == nil {
			continue
		}
		if strings.TrimSpace(info.Session.MicroserviceUUID) != msUUID {
			continue
		}
		if strings.Contains(sessionID, "-hc-") {
			continue
		}
		ids = append(ids, sessionID)
	}
	slices.Sort(ids)
	return ids
}

func enrichMicroserviceStatusExecSessionIDs(rawJSON string, listFn func(msUUID string) []string) string {
	if listFn == nil {
		return rawJSON
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &items); err != nil || len(items) == 0 {
		return rawJSON
	}
	for _, item := range items {
		msUUID, ok := item["id"].(string)
		if !ok || msUUID == "" {
			continue
		}
		containerID, ok := item["containerId"].(string)
		if !ok || containerID == "" {
			continue
		}
		ids := listFn(msUUID)
		if len(ids) > 0 {
			slice := make([]any, len(ids))
			for i, id := range ids {
				slice[i] = id
			}
			item["execSessionIds"] = slice
		} else {
			delete(item, "execSessionIds")
		}
	}
	out, err := json.Marshal(items)
	if err != nil {
		return rawJSON
	}
	return string(out)
}
