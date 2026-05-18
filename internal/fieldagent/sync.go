package fieldagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/serviceaccount"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/eclipse-iofog/agent/internal/volumemount"
)

// loadMicroservices loads microservices from SQLite store or from the controller.
func (fa *FieldAgent) loadMicroservices(fromFile bool) ([]*models.Microservice, error) {
	logging.LogDebug(moduleName, fmt.Sprintf("Start Loading microservices... (fromFile=%v)", fromFile))

	microserviceList := make([]*models.Microservice, 0)

	if fa.NotProvisioned() || !fa.IsControllerConnected(fromFile) {
		return microserviceList, nil
	}

	if fromFile {
		// Load from SQLite store
		logging.LogDebug(moduleName, "Loading microservices from SQLite store")
		stored, err := loadMicroservicesFromStore()
		if err != nil || len(stored) == 0 {
			// Fall back to controller on store miss or error
			logging.LogDebug(moduleName, "Store read returned empty/error, falling back to controller")
			return fa.loadMicroservices(false)
		}
		logging.LogDebug(moduleName, fmt.Sprintf("Loaded %d microservices from store", len(stored)))
		microserviceList = stored
	} else {
		// Load from controller
		logging.LogDebug(moduleName, "Loading microservices from controller")
		ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
		result, err := fa.apiClient.Request(ctx, "microservices", GET, nil, nil)
		cancel()

		if err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error loading microservices from controller: %v", err), err)
			if isCertificateError(err) {
				fa.verificationFailed(err)
				return nil, fmt.Errorf("unable to get microservices due to broken certificate: %w", err)
			}
			return nil, fmt.Errorf("unable to get microservices: %w", err)
		}

		// Parse the controller response
		if msArray, ok := result["microservices"].([]interface{}); ok {
			logging.LogDebug(moduleName, fmt.Sprintf("Received %d microservices from controller", len(msArray)))
			for i, ms := range msArray {
				msMap, ok := ms.(map[string]interface{})
				if !ok {
					continue
				}
				microservice, err := parseMicroservice(msMap)
				if err != nil {
					logging.LogError(moduleName, fmt.Sprintf("Unable to parse microservice at index %d: %v", i, err), err)
					if uuid, ok := msMap["uuid"].(string); ok {
						logging.LogDebug(moduleName, fmt.Sprintf("Failed microservice UUID: %s", uuid))
					}
					continue
				}
				microserviceList = append(microserviceList, microservice)
			}
			logging.LogDebug(moduleName, fmt.Sprintf("Successfully parsed %d out of %d microservices", len(microserviceList), len(msArray)))

			// Persist to SQLite store
			if err := saveMicroservicesToStore(microserviceList); err != nil {
				logging.LogError(moduleName, "Failed to save microservices to store", err)
			}
		} else {
			return nil, fmt.Errorf("error loading microservices from IOFog controller: invalid response format")
		}
	}

	// Store microservices for MicroserviceManagerInterface
	fa.setLatestMicroservices(microserviceList)
	fa.SetCurrentMicroservices(microserviceList)

	// Reconcile service-account token projections for controller-managed microservices.
	if err := serviceaccount.GetInstance().ReconcileManagedMicroservices(microserviceList); err != nil {
		logging.LogError(moduleName, "Failed to reconcile service-account token projections", err)
	}

	// Notify callback if set
	if fa.onMicroservicesUpdate != nil {
		if err := fa.onMicroservicesUpdate(microserviceList); err != nil {
			logging.LogError(moduleName, "Failed to update microservices", err)
		}
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Finished Loading microservices... (count: %d)", len(microserviceList)))
	return microserviceList, nil
}

// parseMicroservice parses a microservice from JSON data
// Matches Java: containerJsonObjectToMicroserviceFunction() - uses "uuid" and "imageId" from JSON
func parseMicroservice(data map[string]interface{}) (*models.Microservice, error) {
	uuid, _ := data["uuid"].(string)
	// Java uses "imageId" from JSON (not "imageName")
	imageID, _ := data["imageId"].(string)

	// Fallback to imageName for backward compatibility
	if imageID == "" {
		imageID, _ = data["imageName"].(string)
	}

	if uuid == "" || imageID == "" {
		return nil, fmt.Errorf("missing required fields: uuid or imageId")
	}

	// Java: new Microservice(jsonObj.getString("uuid"), jsonObj.getString("imageId"))
	// The imageID from JSON is stored as imageName in the Microservice object
	microservice := models.NewMicroservice(uuid, imageID)

	// Parse optional fields
	if config, ok := data["config"].(string); ok {
		microservice.Config = &config
	}
	if runAsUser, ok := data["runAsUser"].(string); ok {
		microservice.RunAsUser = &runAsUser
	}
	if platform, ok := data["platform"].(string); ok {
		microservice.Platform = &platform
	}
	if runtime, ok := data["runtime"].(string); ok {
		microservice.Runtime = &runtime
	}
	if rebuild, ok := data["rebuild"].(bool); ok {
		microservice.Rebuild = rebuild
	}
	if hostNetworkMode, ok := data["hostNetworkMode"].(bool); ok {
		microservice.HostNetworkMode = hostNetworkMode
	}
	if isPrivileged, ok := data["isPrivileged"].(bool); ok {
		microservice.IsPrivileged = isPrivileged
	}
	if registryID, ok := data["registryId"].(float64); ok {
		microservice.RegistryID = int(registryID)
	}
	if schedule, ok := data["schedule"].(float64); ok {
		microservice.Schedule = int(schedule)
	}
	if logSize, ok := data["logSize"].(float64); ok {
		microservice.LogSize = int64(logSize)
	}
	if delete, ok := data["delete"].(bool); ok {
		microservice.Delete = delete
	}
	if deleteWithCleanup, ok := data["deleteWithCleanup"].(bool); ok {
		microservice.DeleteWithCleanup = deleteWithCleanup
	}
	if isRouter, ok := data["isRouter"].(bool); ok {
		microservice.IsRouter = isRouter
		if isRouter {
			cfg := config.GetInstance()
			cfg.SetRouterUUID(uuid)
			cfg.SetRouterInterior(microservice.HostNetworkMode)
		}
	}
	if execEnabled, ok := data["execEnabled"].(bool); ok {
		microservice.ExecEnabled = execEnabled
	}
	if name, ok := data["name"].(string); ok {
		microservice.MicroserviceName = name
	}
	if application, ok := data["application"].(string); ok {
		microservice.ApplicationName = application
	}
	if isNats, ok := data["isNats"].(bool); ok {
		microservice.IsNats = isNats
	}

	// Parse port mappings
	if portMappings, ok := data["portMappings"].([]interface{}); ok {
		microservice.PortMappings = make([]*models.PortMapping, 0, len(portMappings))
		for _, pm := range portMappings {
			if pmMap, ok := pm.(map[string]interface{}); ok {
				var outside, inside int
				var udp bool
				if portExternal, ok := pmMap["portExternal"].(float64); ok {
					outside = int(portExternal)
				}
				if portInternal, ok := pmMap["portInternal"].(float64); ok {
					inside = int(portInternal)
				}
				if isUDP, ok := pmMap["isUdp"].(bool); ok {
					udp = isUDP
				}
				portMapping := models.NewPortMapping(outside, inside, udp)
				microservice.PortMappings = append(microservice.PortMappings, portMapping)
			}
		}
	}

	// Parse volume mappings
	if volumeMappings, ok := data["volumeMappings"].([]interface{}); ok {
		microservice.VolumeMappings = make([]*models.VolumeMapping, 0, len(volumeMappings))
		for _, vm := range volumeMappings {
			if vmMap, ok := vm.(map[string]interface{}); ok {
				volumeMapping := &models.VolumeMapping{}
				if hostDestination, ok := vmMap["hostDestination"].(string); ok {
					volumeMapping.HostDestination = hostDestination
				}
				if containerDestination, ok := vmMap["containerDestination"].(string); ok {
					volumeMapping.ContainerDestination = containerDestination
				}
				if accessMode, ok := vmMap["accessMode"].(string); ok {
					volumeMapping.AccessMode = accessMode
				}
				if typeStr, ok := vmMap["type"].(string); ok {
					switch typeStr {
					case "volumeMount":
						volumeMapping.Type = models.VolumeMappingTypeVolumeMount
					case "volume":
						volumeMapping.Type = models.VolumeMappingTypeVolume
					default:
						volumeMapping.Type = models.VolumeMappingTypeBind
					}
				}
				microservice.VolumeMappings = append(microservice.VolumeMappings, volumeMapping)
			}
		}
	}

	// Parse env vars (matching Java: envVarsValue.getJsonArray("env"))
	if envVars, ok := data["env"].([]interface{}); ok {
		microservice.EnvVars = make([]*models.EnvVar, 0, len(envVars))
		for _, env := range envVars {
			if envMap, ok := env.(map[string]interface{}); ok {
				envVar := &models.EnvVar{}
				if key, ok := envMap["key"].(string); ok {
					envVar.Key = key
				}
				if value, ok := envMap["value"].(string); ok {
					envVar.Value = value
				}
				// Only add if both key and value are present
				if envVar.Key != "" || envVar.Value != "" {
					microservice.EnvVars = append(microservice.EnvVars, envVar)
				}
			}
		}
	}

	// Parse args
	if args, ok := data["cmd"].([]interface{}); ok {
		microservice.Args = make([]string, 0, len(args))
		for _, arg := range args {
			if argStr, ok := arg.(string); ok {
				microservice.Args = append(microservice.Args, argStr)
			}
		}
	}

	// Parse memory limit (matching Java: getJsonNumber("memoryLimit").longValue())
	if memoryLimit, ok := data["memoryLimit"].(float64); ok {
		memoryLimitBytes := int64(memoryLimit)
		microservice.MemoryLimit = &memoryLimitBytes
	}

	// Parse cdiDevices (matching Java: getStringList(cdiDevsValue))
	if cdiDevices, ok := data["cdiDevices"].([]interface{}); ok {
		microservice.CdiDevs = make([]string, 0, len(cdiDevices))
		for _, device := range cdiDevices {
			if deviceStr, ok := device.(string); ok {
				microservice.CdiDevs = append(microservice.CdiDevs, deviceStr)
			}
		}
	}

	// Parse annotations (matching Java: jsonObj.getString("annotations"))
	if annotations, ok := data["annotations"].(string); ok && annotations != "" {
		microservice.Annotations = &annotations
	}

	// Parse capAdd (matching Java: getStringList(capAddValue))
	if capAdd, ok := data["capAdd"].([]interface{}); ok {
		microservice.CapAdd = make([]string, 0, len(capAdd))
		for _, cap := range capAdd {
			if capStr, ok := cap.(string); ok {
				microservice.CapAdd = append(microservice.CapAdd, capStr)
			}
		}
	}

	// Parse capDrop (matching Java: getStringList(capDropValue))
	if capDrop, ok := data["capDrop"].([]interface{}); ok {
		microservice.CapDrop = make([]string, 0, len(capDrop))
		for _, cap := range capDrop {
			if capStr, ok := cap.(string); ok {
				microservice.CapDrop = append(microservice.CapDrop, capStr)
			}
		}
	}

	// Parse extraHosts (matching Java: getStringList(extraHostsValue))
	if extraHosts, ok := data["extraHosts"].([]interface{}); ok {
		microservice.ExtraHosts = make([]string, 0, len(extraHosts))
		for _, host := range extraHosts {
			if hostStr, ok := host.(string); ok {
				microservice.ExtraHosts = append(microservice.ExtraHosts, hostStr)
			}
		}
	}

	// Parse pidMode (matching Java: jsonObj.getString("pidMode"))
	if pidMode, ok := data["pidMode"].(string); ok && pidMode != "" {
		microservice.PidMode = &pidMode
	}

	// Parse ipcMode (matching Java: jsonObj.getString("ipcMode"))
	if ipcMode, ok := data["ipcMode"].(string); ok && ipcMode != "" {
		microservice.IpcMode = &ipcMode
	}

	// Parse cpuSetCpus (matching Java: jsonObj.getString("cpuSetCpus"))
	if cpuSetCpus, ok := data["cpuSetCpus"].(string); ok && cpuSetCpus != "" {
		microservice.CPUSetCpus = &cpuSetCpus
	}

	// Parse healthCheck (matching Java: healthcheckValue.getJsonObject("healthCheck"))
	if healthCheck, ok := data["healthCheck"].(map[string]interface{}); ok {
		healthcheck := &models.Healthcheck{}

		// Parse test (array of strings)
		if test, ok := healthCheck["test"].([]interface{}); ok {
			healthcheck.Test = make([]string, 0, len(test))
			for _, t := range test {
				if testStr, ok := t.(string); ok {
					healthcheck.Test = append(healthcheck.Test, testStr)
				}
			}
		}

		// Parse numeric fields (can be null in Java)
		if interval, ok := healthCheck["interval"].(float64); ok {
			intervalVal := int64(interval)
			healthcheck.Interval = &intervalVal
		}
		if timeout, ok := healthCheck["timeout"].(float64); ok {
			timeoutVal := int64(timeout)
			healthcheck.Timeout = &timeoutVal
		}
		if startPeriod, ok := healthCheck["startPeriod"].(float64); ok {
			startPeriodVal := int64(startPeriod)
			healthcheck.StartPeriod = &startPeriodVal
		}
		if startInterval, ok := healthCheck["startInterval"].(float64); ok {
			startIntervalVal := int64(startInterval)
			healthcheck.StartInterval = &startIntervalVal
		}
		if retries, ok := healthCheck["retries"].(float64); ok {
			retriesVal := int(retries)
			healthcheck.Retries = &retriesVal
		}

		microservice.Healthcheck = healthcheck
	}

	// Parse serviceAccount (name/roleRef/rules) for token claim normalization.
	if saData, ok := data["serviceAccount"].(map[string]interface{}); ok {
		sa := &models.ServiceAccount{}
		if name, ok := saData["name"].(string); ok {
			sa.Name = name
		}
		if roleRef, ok := saData["roleRef"].(map[string]interface{}); ok {
			if kind, ok := roleRef["kind"].(string); ok {
				sa.RoleRef.Kind = kind
			}
			if roleName, ok := roleRef["name"].(string); ok {
				sa.RoleRef.Name = roleName
			}
		}
		if rules, ok := saData["rules"].([]interface{}); ok {
			sa.Rules = make([]models.ServiceAccountRule, 0, len(rules))
			for _, rawRule := range rules {
				ruleMap, ok := rawRule.(map[string]interface{})
				if !ok {
					continue
				}
				rule := models.ServiceAccountRule{
					APIGroups:     parseStringArray(ruleMap["apiGroups"]),
					Resources:     parseStringArray(ruleMap["resources"]),
					Verbs:         parseStringArray(ruleMap["verbs"]),
					ResourceNames: parseStringArray(ruleMap["resourceNames"]),
				}
				sort.Strings(rule.APIGroups)
				sort.Strings(rule.Resources)
				sort.Strings(rule.ResourceNames)
				rule.Verbs = models.CanonicalizeVerbs(rule.Verbs)
				sa.Rules = append(sa.Rules, rule)
			}
		}
		microservice.ServiceAccount = sa
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Successfully parsed microservice: uuid=%s, imageId=%s", uuid, imageID))
	return microservice, nil
}

func parseStringArray(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			result = append(result, s)
		}
	}
	return result
}

// loadRegistries loads registries from SQLite store or from the controller.
func (fa *FieldAgent) loadRegistries(fromFile bool) error {
	logging.LogDebug(moduleName, "get registries")

	if fa.NotProvisioned() || !fa.IsControllerConnected(fromFile) {
		return nil
	}

	var registries []*models.Registry

	if fromFile {
		// Load from SQLite store
		stored, err := loadRegistriesFromStore()
		if err != nil || len(stored) == 0 {
			// Fall back to controller on store miss or error
			return fa.loadRegistries(false)
		}
		registries = stored
		logging.LogDebug(moduleName, fmt.Sprintf("Loaded %d registries from store", len(registries)))
	} else {
		// Load from controller
		ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
		result, err := fa.apiClient.Request(ctx, "registries", GET, nil, nil)
		cancel()

		if err != nil {
			if isCertificateError(err) {
				fa.verificationFailed(err)
				return fmt.Errorf("unable to get registries due to broken certificate: %w", err)
			}
			return fmt.Errorf("unable to get registries: %w", err)
		}

		registries = make([]*models.Registry, 0)
		if regArray, ok := result["registries"].([]interface{}); ok {
			for _, reg := range regArray {
				if regMap, ok := reg.(map[string]interface{}); ok {
					registries = append(registries, parseRegistry(regMap))
				}
			}
		} else {
			return fmt.Errorf("error loading registries from IOFog controller: invalid response format")
		}

		// Persist to SQLite store
		if err := saveRegistriesToStore(registries); err != nil {
			logging.LogError(moduleName, "Failed to save registries to store", err)
		}
	}

	if len(registries) == 0 {
		logging.LogInfo(moduleName, "Registries list is empty")
	}

	// Store registries for MicroserviceManagerInterface
	fa.setRegistries(registries)

	// Notify callback if set
	if fa.onRegistriesUpdate != nil {
		if err := fa.onRegistriesUpdate(registries); err != nil {
			logging.LogError(moduleName, "Failed to update registries", err)
		}
	}

	logging.LogDebug(moduleName, "Finished get registries")
	return nil
}

// parseRegistry parses a registry from JSON data
func parseRegistry(data map[string]interface{}) *models.Registry {
	builder := models.NewRegistryBuilder()

	var id int
	var url string
	var isPublic bool
	var userName, password, userEmail string

	if idVal, ok := data["id"].(float64); ok {
		id = int(idVal)
	}
	if urlVal, ok := data["url"].(string); ok {
		url = urlVal
	}
	if isPublicVal, ok := data["isPublic"].(bool); ok {
		isPublic = isPublicVal
	}

	if !isPublic {
		if userNameVal, ok := data["username"].(string); ok {
			userName = userNameVal
		}
		if passwordVal, ok := data["password"].(string); ok {
			password = passwordVal
		}
		if userEmailVal, ok := data["userEmail"].(string); ok {
			userEmail = userEmailVal
		}
	}

	return builder.SetID(id).SetURL(url).SetIsPublic(isPublic).
		SetUserName(userName).SetPassword(password).SetUserEmail(userEmail).Build()
}

// processMicroserviceConfig processes microservice configurations
func (fa *FieldAgent) processMicroserviceConfig(microservices []*models.Microservice) error {
	logging.LogDebug(moduleName, "Start process microservice configuration")

	configs := make(map[string]string)
	fa.containerConfigMu.Lock()
	for _, microservice := range microservices {
		if microservice.Config != nil {
			configStr := *microservice.Config
			configs[microservice.MicroserviceUUID] = configStr
			fa.containerConfigMap[microservice.MicroserviceUUID] = configStr
		}
	}
	fa.containerConfigMu.Unlock()

	// Notify callback if set
	if fa.onConfigsUpdate != nil {
		if err := fa.onConfigsUpdate(configs); err != nil {
			return fmt.Errorf("failed to update configs: %w", err)
		}
	}

	logging.LogDebug(moduleName, "Finished process microservice configuration")
	return nil
}

// loadVolumeMounts loads volume mounts from controller
// Matches Java: loadVolumeMounts() - catches exceptions and continues
func (fa *FieldAgent) loadVolumeMounts() error {
	logging.LogDebug(moduleName, "Start loading volume mounts")

	// Use defer/recover to catch any panics (matching Java try-catch behavior)
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(moduleName, fmt.Sprintf("Panic in loadVolumeMounts: %v", r), fmt.Errorf("%v", r))
		}
	}()

	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	defer cancel()

	result, err := fa.apiClient.Request(ctx, "volumeMounts", GET, nil, nil)
	if err != nil {
		// Log error but don't fail startup (matching Java: catch Exception and log)
		logging.LogError(moduleName, "Unable to process volume mount changes", err)
		logging.LogDebug(moduleName, "Finished loading volume mounts (with error)")
		return nil // Return nil to continue execution
	}

	if volumeMounts, ok := result["volumeMounts"].([]interface{}); ok {
		// Process volume mount changes via VolumeMountManager
		// Wrap in recover to catch any panics
		func() {
			defer func() {
				if r := recover(); r != nil {
					logging.LogError(moduleName, fmt.Sprintf("Panic in ProcessVolumeMountChanges: %v", r), fmt.Errorf("%v", r))
				}
			}()
			volumemount.GetInstance().ProcessVolumeMountChanges(volumeMounts)
		}()
		logging.LogDebug(moduleName, fmt.Sprintf("Loaded %d volume mounts", len(volumeMounts)))
	} else {
		logging.LogDebug(moduleName, "No volumeMounts in response")
	}

	logging.LogInfo(moduleName, "Finished loading volume mounts")
	return nil
}
