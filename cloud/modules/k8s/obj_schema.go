package k8s

import (
	"go.starlark.net/starlark"
)

// --- Metadata fields shared by all top-level resources ---
var metadataFields = map[string]*FieldSpec{
	"name":        {JSONKey: "name", Typ: FieldString, Required: true},
	"namespace":   {JSONKey: "namespace", Typ: FieldString},
	"labels":      {JSONKey: "labels", Typ: FieldDict},
	"annotations": {JSONKey: "annotations", Typ: FieldDict},
}

// mergeFields creates a new field map from metadata + spec fields.
func mergeFields(specFields map[string]*FieldSpec) map[string]*FieldSpec {
	result := make(map[string]*FieldSpec, len(metadataFields)+len(specFields))
	for k, v := range metadataFields {
		result[k] = v
	}
	for k, v := range specFields {
		result[k] = v
	}
	return result
}

// --- Top-level resource schemas ---

var podSchema = &ResourceSchema{
	Kind:       "Pod",
	APIVersion: "v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"containers":      {JSONKey: "containers", Typ: FieldList, SpecKey: true, Required: true},
		"init_containers": {JSONKey: "initContainers", Typ: FieldList, SpecKey: true},
		"volumes":         {JSONKey: "volumes", Typ: FieldList, SpecKey: true},
		"restart_policy":  {JSONKey: "restartPolicy", Typ: FieldString, SpecKey: true, DefaultVal: "Always"},
		"node_selector":   {JSONKey: "nodeSelector", Typ: FieldDict, SpecKey: true},
		"tolerations":     {JSONKey: "tolerations", Typ: FieldList, SpecKey: true},
		"affinity":        {JSONKey: "affinity", Typ: FieldKubeObject, SpecKey: true},
		"service_account": {JSONKey: "serviceAccountName", Typ: FieldString, SpecKey: true},
		"host_network":    {JSONKey: "hostNetwork", Typ: FieldBool, SpecKey: true},
		"dns_policy":      {JSONKey: "dnsPolicy", Typ: FieldString, SpecKey: true},
		"security_context": {JSONKey: "securityContext", Typ: FieldKubeObject, SpecKey: true},
	}),
}

var deploymentSchema = &ResourceSchema{
	Kind:       "Deployment",
	APIVersion: "apps/v1",
	Fields: mergeWithPodFields(map[string]*FieldSpec{
		"replicas":         {JSONKey: "replicas", Typ: FieldInt, SpecKey: true, DefaultVal: 1},
		"selector":         {JSONKey: "selector", Typ: FieldDict, SpecKey: true},
		"template":         {JSONKey: "template", Typ: FieldKubeObject, SpecKey: true},
		"strategy":         {JSONKey: "strategy", Typ: FieldDict, SpecKey: true},
		"min_ready_seconds": {JSONKey: "minReadySeconds", Typ: FieldInt, SpecKey: true},
		"revision_history":  {JSONKey: "revisionHistoryLimit", Typ: FieldInt, SpecKey: true},
	}),
}

var statefulSetSchema = &ResourceSchema{
	Kind:       "StatefulSet",
	APIVersion: "apps/v1",
	Fields: mergeWithPodFields(map[string]*FieldSpec{
		"replicas":           {JSONKey: "replicas", Typ: FieldInt, SpecKey: true, DefaultVal: 1},
		"selector":           {JSONKey: "selector", Typ: FieldDict, SpecKey: true},
		"template":           {JSONKey: "template", Typ: FieldKubeObject, SpecKey: true},
		"service_name":       {JSONKey: "serviceName", Typ: FieldString, SpecKey: true},
		"volume_claim_templates": {JSONKey: "volumeClaimTemplates", Typ: FieldList, SpecKey: true},
		"pod_management_policy": {JSONKey: "podManagementPolicy", Typ: FieldString, SpecKey: true},
	}),
}

var daemonSetSchema = &ResourceSchema{
	Kind:       "DaemonSet",
	APIVersion: "apps/v1",
	Fields: mergeWithPodFields(map[string]*FieldSpec{
		"selector":          {JSONKey: "selector", Typ: FieldDict, SpecKey: true},
		"template":          {JSONKey: "template", Typ: FieldKubeObject, SpecKey: true},
		"update_strategy":   {JSONKey: "updateStrategy", Typ: FieldDict, SpecKey: true},
		"min_ready_seconds": {JSONKey: "minReadySeconds", Typ: FieldInt, SpecKey: true},
	}),
}

var jobSchema = &ResourceSchema{
	Kind:       "Job",
	APIVersion: "batch/v1",
	Fields: mergeWithPodFields(map[string]*FieldSpec{
		"template":            {JSONKey: "template", Typ: FieldKubeObject, SpecKey: true},
		"completions":         {JSONKey: "completions", Typ: FieldInt, SpecKey: true},
		"parallelism":         {JSONKey: "parallelism", Typ: FieldInt, SpecKey: true},
		"backoff_limit":       {JSONKey: "backoffLimit", Typ: FieldInt, SpecKey: true, DefaultVal: 6},
		"active_deadline":     {JSONKey: "activeDeadlineSeconds", Typ: FieldInt, SpecKey: true},
		"ttl_after_finished":  {JSONKey: "ttlSecondsAfterFinished", Typ: FieldInt, SpecKey: true},
	}),
}

var cronJobSchema = &ResourceSchema{
	Kind:       "CronJob",
	APIVersion: "batch/v1",
	Fields: mergeWithPodFields(map[string]*FieldSpec{
		"schedule":           {JSONKey: "schedule", Typ: FieldString, SpecKey: true, Required: true},
		"job_template":       {JSONKey: "jobTemplate", Typ: FieldKubeObject, SpecKey: true},
		"concurrency_policy": {JSONKey: "concurrencyPolicy", Typ: FieldString, SpecKey: true, DefaultVal: "Allow"},
		"suspend":            {JSONKey: "suspend", Typ: FieldBool, SpecKey: true},
		"history_limit":      {JSONKey: "successfulJobsHistoryLimit", Typ: FieldInt, SpecKey: true},
		"failed_history":     {JSONKey: "failedJobsHistoryLimit", Typ: FieldInt, SpecKey: true},
	}),
}

var serviceSchema = &ResourceSchema{
	Kind:       "Service",
	APIVersion: "v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"ports":            {JSONKey: "ports", Typ: FieldList, SpecKey: true},
		"selector":         {JSONKey: "selector", Typ: FieldDict, SpecKey: true},
		"type":             {JSONKey: "type", Typ: FieldString, SpecKey: true, DefaultVal: "ClusterIP"},
		"cluster_ip":       {JSONKey: "clusterIP", Typ: FieldString, SpecKey: true},
		"external_ips":     {JSONKey: "externalIPs", Typ: FieldList, SpecKey: true},
		"session_affinity": {JSONKey: "sessionAffinity", Typ: FieldString, SpecKey: true},
	}),
}

var ingressSchema = &ResourceSchema{
	Kind:       "Ingress",
	APIVersion: "networking.k8s.io/v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"rules":             {JSONKey: "rules", Typ: FieldList, SpecKey: true},
		"tls":               {JSONKey: "tls", Typ: FieldList, SpecKey: true},
		"default_backend":   {JSONKey: "defaultBackend", Typ: FieldDict, SpecKey: true},
		"ingress_class_name": {JSONKey: "ingressClassName", Typ: FieldString, SpecKey: true},
	}),
}

var configMapSchema = &ResourceSchema{
	Kind:       "ConfigMap",
	APIVersion: "v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"data":        {JSONKey: "data", Typ: FieldDict},
		"binary_data": {JSONKey: "binaryData", Typ: FieldDict},
		"immutable":   {JSONKey: "immutable", Typ: FieldBool},
	}),
}

var secretSchema = &ResourceSchema{
	Kind:       "Secret",
	APIVersion: "v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"data":         {JSONKey: "data", Typ: FieldDict},
		"string_data":  {JSONKey: "stringData", Typ: FieldDict},
		"type":         {JSONKey: "type", Typ: FieldString, DefaultVal: "Opaque"},
		"immutable":    {JSONKey: "immutable", Typ: FieldBool},
	}),
}

var namespaceSchema = &ResourceSchema{
	Kind:       "Namespace",
	APIVersion: "v1",
	Fields: map[string]*FieldSpec{
		"name":        {JSONKey: "name", Typ: FieldString, Required: true},
		"labels":      {JSONKey: "labels", Typ: FieldDict},
		"annotations": {JSONKey: "annotations", Typ: FieldDict},
	},
}

var pvcSchema = &ResourceSchema{
	Kind:       "PersistentVolumeClaim",
	APIVersion: "v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"access_modes":  {JSONKey: "accessModes", Typ: FieldList, SpecKey: true},
		"resources":     {JSONKey: "resources", Typ: FieldDict, SpecKey: true},
		"storage":       {JSONKey: "storage", Typ: FieldString}, // convenience, expanded to spec.resources.requests.storage
		"storage_class": {JSONKey: "storageClassName", Typ: FieldString, SpecKey: true},
		"volume_mode":   {JSONKey: "volumeMode", Typ: FieldString, SpecKey: true},
	}),
}

var serviceAccountSchema = &ResourceSchema{
	Kind:       "ServiceAccount",
	APIVersion: "v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"secrets":                      {JSONKey: "secrets", Typ: FieldList},
		"image_pull_secrets":           {JSONKey: "imagePullSecrets", Typ: FieldList},
		"automount_service_account_token": {JSONKey: "automountServiceAccountToken", Typ: FieldBool},
	}),
}

var hpaSchema = &ResourceSchema{
	Kind:       "HorizontalPodAutoscaler",
	APIVersion: "autoscaling/v2",
	Fields: mergeFields(map[string]*FieldSpec{
		"scale_target_ref": {JSONKey: "scaleTargetRef", Typ: FieldDict, SpecKey: true, Required: true},
		"min_replicas":     {JSONKey: "minReplicas", Typ: FieldInt, SpecKey: true, DefaultVal: 1},
		"max_replicas":     {JSONKey: "maxReplicas", Typ: FieldInt, SpecKey: true, Required: true},
		"metrics":          {JSONKey: "metrics", Typ: FieldList, SpecKey: true},
	}),
}

var roleSchema = &ResourceSchema{
	Kind:       "Role",
	APIVersion: "rbac.authorization.k8s.io/v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"rules": {JSONKey: "rules", Typ: FieldList},
	}),
}

var clusterRoleSchema = &ResourceSchema{
	Kind:       "ClusterRole",
	APIVersion: "rbac.authorization.k8s.io/v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"rules":             {JSONKey: "rules", Typ: FieldList},
		"aggregation_rule":  {JSONKey: "aggregationRule", Typ: FieldDict},
	}),
}

var roleBindingSchema = &ResourceSchema{
	Kind:       "RoleBinding",
	APIVersion: "rbac.authorization.k8s.io/v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"role_ref": {JSONKey: "roleRef", Typ: FieldDict, Required: true},
		"subjects": {JSONKey: "subjects", Typ: FieldList},
	}),
}

var clusterRoleBindingSchema = &ResourceSchema{
	Kind:       "ClusterRoleBinding",
	APIVersion: "rbac.authorization.k8s.io/v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"role_ref": {JSONKey: "roleRef", Typ: FieldDict, Required: true},
		"subjects": {JSONKey: "subjects", Typ: FieldList},
	}),
}

var networkPolicySchema = &ResourceSchema{
	Kind:       "NetworkPolicy",
	APIVersion: "networking.k8s.io/v1",
	Fields: mergeFields(map[string]*FieldSpec{
		"pod_selector":  {JSONKey: "podSelector", Typ: FieldDict, SpecKey: true},
		"ingress":       {JSONKey: "ingress", Typ: FieldList, SpecKey: true},
		"egress":        {JSONKey: "egress", Typ: FieldList, SpecKey: true},
		"policy_types":  {JSONKey: "policyTypes", Typ: FieldList, SpecKey: true},
	}),
}

// --- Pod fields for workload flattening ---
// These fields are accepted by workload constructors (Deployment, StatefulSet, etc.)
// and consumed by autoTemplate() to build the pod template automatically.
// They are NOT SpecKey — they never serialize directly to spec.

var podTemplateFields = map[string]*FieldSpec{
	"containers":           {JSONKey: "containers", Typ: FieldList},
	"init_containers":      {JSONKey: "initContainers", Typ: FieldList},
	"volumes":              {JSONKey: "volumes", Typ: FieldList},
	"restart_policy":       {JSONKey: "restartPolicy", Typ: FieldString},
	"node_selector":        {JSONKey: "nodeSelector", Typ: FieldDict},
	"tolerations":          {JSONKey: "tolerations", Typ: FieldList},
	"affinity":             {JSONKey: "affinity", Typ: FieldKubeObject},
	"service_account":      {JSONKey: "serviceAccountName", Typ: FieldString},
	"host_network":         {JSONKey: "hostNetwork", Typ: FieldBool},
	"dns_policy":           {JSONKey: "dnsPolicy", Typ: FieldString},
	"security_context":     {JSONKey: "securityContext", Typ: FieldKubeObject},
	"template_labels":      {JSONKey: "templateLabels", Typ: FieldDict},
	"template_annotations": {JSONKey: "templateAnnotations", Typ: FieldDict},
}

// mergeWithPodFields adds podTemplateFields to a spec field map for workload schemas.
func mergeWithPodFields(specFields map[string]*FieldSpec) map[string]*FieldSpec {
	result := mergeFields(specFields)
	for k, v := range podTemplateFields {
		result[k] = v
	}
	return result
}

// --- Sub-object schemas ---

var podSpecSchema = &ResourceSchema{
	Kind:        "PodSpec",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"containers":       {JSONKey: "containers", Typ: FieldList, Required: true},
		"init_containers":  {JSONKey: "initContainers", Typ: FieldList},
		"volumes":          {JSONKey: "volumes", Typ: FieldList},
		"restart_policy":   {JSONKey: "restartPolicy", Typ: FieldString, DefaultVal: "Always"},
		"node_selector":    {JSONKey: "nodeSelector", Typ: FieldDict},
		"tolerations":      {JSONKey: "tolerations", Typ: FieldList},
		"affinity":         {JSONKey: "affinity", Typ: FieldKubeObject},
		"service_account":  {JSONKey: "serviceAccountName", Typ: FieldString},
		"host_network":     {JSONKey: "hostNetwork", Typ: FieldBool},
		"dns_policy":       {JSONKey: "dnsPolicy", Typ: FieldString},
		"security_context": {JSONKey: "securityContext", Typ: FieldKubeObject},
	},
}

var podTemplateSchema = &ResourceSchema{
	Kind:        "PodTemplate",
	IsSubObject: true,
	IsTemplate:  true,
	Fields: map[string]*FieldSpec{
		"labels":           {JSONKey: "labels", Typ: FieldDict, MetaKey: true},
		"annotations":      {JSONKey: "annotations", Typ: FieldDict, MetaKey: true},
		"containers":       {JSONKey: "containers", Typ: FieldList, Required: true},
		"init_containers":  {JSONKey: "initContainers", Typ: FieldList},
		"volumes":          {JSONKey: "volumes", Typ: FieldList},
		"restart_policy":   {JSONKey: "restartPolicy", Typ: FieldString},
		"node_selector":    {JSONKey: "nodeSelector", Typ: FieldDict},
		"tolerations":      {JSONKey: "tolerations", Typ: FieldList},
		"affinity":         {JSONKey: "affinity", Typ: FieldKubeObject},
		"service_account":  {JSONKey: "serviceAccountName", Typ: FieldString},
		"host_network":     {JSONKey: "hostNetwork", Typ: FieldBool},
		"dns_policy":       {JSONKey: "dnsPolicy", Typ: FieldString},
		"security_context": {JSONKey: "securityContext", Typ: FieldKubeObject},
	},
}

var containerSchema = &ResourceSchema{
	Kind:        "Container",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"name":             {JSONKey: "name", Typ: FieldString, Required: true},
		"image":            {JSONKey: "image", Typ: FieldString, Required: true},
		"command":          {JSONKey: "command", Typ: FieldList},
		"args":             {JSONKey: "args", Typ: FieldList},
		"ports":            {JSONKey: "ports", Typ: FieldList},
		"env":              {JSONKey: "env", Typ: FieldList},
		"env_from":         {JSONKey: "envFrom", Typ: FieldList},
		"volume_mounts":    {JSONKey: "volumeMounts", Typ: FieldList},
		"resources":        {JSONKey: "resources", Typ: FieldKubeObject},
		"liveness_probe":   {JSONKey: "livenessProbe", Typ: FieldKubeObject},
		"readiness_probe":  {JSONKey: "readinessProbe", Typ: FieldKubeObject},
		"startup_probe":    {JSONKey: "startupProbe", Typ: FieldKubeObject},
		"working_dir":      {JSONKey: "workingDir", Typ: FieldString},
		"image_pull_policy": {JSONKey: "imagePullPolicy", Typ: FieldString},
		"security_context": {JSONKey: "securityContext", Typ: FieldKubeObject},
	},
}

var containerPortSchema = &ResourceSchema{
	Kind:        "ContainerPort",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"container_port": {JSONKey: "containerPort", Typ: FieldInt, Required: true},
		"name":           {JSONKey: "name", Typ: FieldString},
		"protocol":       {JSONKey: "protocol", Typ: FieldString, DefaultVal: "TCP"},
		"host_port":      {JSONKey: "hostPort", Typ: FieldInt},
	},
}

var volumeSchema = &ResourceSchema{
	Kind:        "Volume",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"name":          {JSONKey: "name", Typ: FieldString, Required: true},
		"empty_dir":     {JSONKey: "emptyDir", Typ: FieldDict},
		"config_map":    {JSONKey: "configMap", Typ: FieldDict},
		"secret":        {JSONKey: "secret", Typ: FieldDict},
		"pvc":           {JSONKey: "persistentVolumeClaim", Typ: FieldDict},
		"host_path":     {JSONKey: "hostPath", Typ: FieldDict},
	},
}

var volumeMountSchema = &ResourceSchema{
	Kind:        "VolumeMount",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"name":       {JSONKey: "name", Typ: FieldString, Required: true},
		"mount_path": {JSONKey: "mountPath", Typ: FieldString, Required: true},
		"sub_path":   {JSONKey: "subPath", Typ: FieldString},
		"read_only":  {JSONKey: "readOnly", Typ: FieldBool},
	},
}

var envVarSchema = &ResourceSchema{
	Kind:        "EnvVar",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"name":       {JSONKey: "name", Typ: FieldString, Required: true},
		"value":      {JSONKey: "value", Typ: FieldString},
		"value_from": {JSONKey: "valueFrom", Typ: FieldDict},
	},
}

var envFromSchema = &ResourceSchema{
	Kind:        "EnvFrom",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"config_map_ref": {JSONKey: "configMapRef", Typ: FieldDict},
		"secret_ref":     {JSONKey: "secretRef", Typ: FieldDict},
		"prefix":         {JSONKey: "prefix", Typ: FieldString},
	},
}

var resourceRequirementsSchema = &ResourceSchema{
	Kind:        "ResourceRequirements",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"requests": {JSONKey: "requests", Typ: FieldDict},
		"limits":   {JSONKey: "limits", Typ: FieldDict},
	},
}

var probeSchema = &ResourceSchema{
	Kind:        "Probe",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"http_get":              {JSONKey: "httpGet", Typ: FieldDict},
		"tcp_socket":            {JSONKey: "tcpSocket", Typ: FieldDict},
		"exec":                  {JSONKey: "exec", Typ: FieldDict},
		"initial_delay_seconds": {JSONKey: "initialDelaySeconds", Typ: FieldInt},
		"period_seconds":        {JSONKey: "periodSeconds", Typ: FieldInt, DefaultVal: 10},
		"timeout_seconds":       {JSONKey: "timeoutSeconds", Typ: FieldInt, DefaultVal: 1},
		"success_threshold":     {JSONKey: "successThreshold", Typ: FieldInt, DefaultVal: 1},
		"failure_threshold":     {JSONKey: "failureThreshold", Typ: FieldInt, DefaultVal: 3},
	},
}

var servicePortSchema = &ResourceSchema{
	Kind:        "ServicePort",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"port":        {JSONKey: "port", Typ: FieldInt, Required: true},
		"target_port": {JSONKey: "targetPort", Typ: FieldInt},
		"name":        {JSONKey: "name", Typ: FieldString},
		"protocol":    {JSONKey: "protocol", Typ: FieldString, DefaultVal: "TCP"},
		"node_port":   {JSONKey: "nodePort", Typ: FieldInt},
	},
}

var ingressRuleSchema = &ResourceSchema{
	Kind:        "IngressRule",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"host":  {JSONKey: "host", Typ: FieldString},
		"paths": {JSONKey: "http", Typ: FieldDict},
	},
}

var ingressPathSchema = &ResourceSchema{
	Kind:        "IngressPath",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"path":      {JSONKey: "path", Typ: FieldString, Required: true},
		"path_type": {JSONKey: "pathType", Typ: FieldString, DefaultVal: "Prefix"},
		"backend":   {JSONKey: "backend", Typ: FieldDict, Required: true},
	},
}

var tolerationSchema = &ResourceSchema{
	Kind:        "Toleration",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"key":       {JSONKey: "key", Typ: FieldString},
		"operator":  {JSONKey: "operator", Typ: FieldString, DefaultVal: "Equal"},
		"value":     {JSONKey: "value", Typ: FieldString},
		"effect":    {JSONKey: "effect", Typ: FieldString},
		"toleration_seconds": {JSONKey: "tolerationSeconds", Typ: FieldInt},
	},
}

var affinitySchema = &ResourceSchema{
	Kind:        "Affinity",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"node_affinity":     {JSONKey: "nodeAffinity", Typ: FieldDict},
		"pod_affinity":      {JSONKey: "podAffinity", Typ: FieldDict},
		"pod_anti_affinity": {JSONKey: "podAntiAffinity", Typ: FieldDict},
	},
}

var policyRuleSchema = &ResourceSchema{
	Kind:        "PolicyRule",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"api_groups": {JSONKey: "apiGroups", Typ: FieldList, Required: true},
		"resources":  {JSONKey: "resources", Typ: FieldList, Required: true},
		"verbs":      {JSONKey: "verbs", Typ: FieldList, Required: true},
	},
}

var subjectSchema = &ResourceSchema{
	Kind:        "Subject",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"kind":      {JSONKey: "kind", Typ: FieldString, Required: true},
		"name":      {JSONKey: "name", Typ: FieldString, Required: true},
		"namespace": {JSONKey: "namespace", Typ: FieldString},
		"api_group": {JSONKey: "apiGroup", Typ: FieldString},
	},
}

var securityContextSchema = &ResourceSchema{
	Kind:        "SecurityContext",
	IsSubObject: true,
	Fields: map[string]*FieldSpec{
		"run_as_user":     {JSONKey: "runAsUser", Typ: FieldInt},
		"run_as_group":    {JSONKey: "runAsGroup", Typ: FieldInt},
		"run_as_non_root": {JSONKey: "runAsNonRoot", Typ: FieldBool},
		"read_only_root":  {JSONKey: "readOnlyRootFilesystem", Typ: FieldBool},
		"privileged":      {JSONKey: "privileged", Typ: FieldBool},
		"capabilities":    {JSONKey: "capabilities", Typ: FieldDict},
	},
}

// --- Schema registry ---

var allSchemas = map[string]*ResourceSchema{
	"pod":                        podSchema,
	"deployment":                 deploymentSchema,
	"stateful_set":               statefulSetSchema,
	"daemon_set":                 daemonSetSchema,
	"job":                        jobSchema,
	"cron_job":                   cronJobSchema,
	"service":                    serviceSchema,
	"ingress":                    ingressSchema,
	"config_map":                 configMapSchema,
	"secret":                     secretSchema,
	"namespace":                  namespaceSchema,
	"persistent_volume_claim":    pvcSchema,
	"service_account":            serviceAccountSchema,
	"horizontal_pod_autoscaler":  hpaSchema,
	"role":                       roleSchema,
	"cluster_role":               clusterRoleSchema,
	"role_binding":               roleBindingSchema,
	"cluster_role_binding":       clusterRoleBindingSchema,
	"network_policy":             networkPolicySchema,
	"pod_spec":                   podSpecSchema,
	"pod_template":               podTemplateSchema,
	"container":                  containerSchema,
	"container_port":             containerPortSchema,
	"volume":                     volumeSchema,
	"volume_mount":               volumeMountSchema,
	"env_var":                    envVarSchema,
	"env_from":                   envFromSchema,
	"resource_requirements":      resourceRequirementsSchema,
	"probe":                      probeSchema,
	"service_port":               servicePortSchema,
	"ingress_rule":               ingressRuleSchema,
	"ingress_path":               ingressPathSchema,
	"toleration":                 tolerationSchema,
	"affinity":                   affinitySchema,
	"policy_rule":                policyRuleSchema,
	"subject":                    subjectSchema,
	"security_context":           securityContextSchema,
}

// makeObjConstructor creates a Starlark builtin for a k8s.obj constructor.
func makeObjConstructor(name string, schema *ResourceSchema) *starlark.Builtin {
	return starlark.NewBuiltin("k8s.obj."+name, func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return newKubeResource(schema, args, kwargs)
	})
}

// ObjConstructors returns all k8s.obj constructors as a StringDict.
func ObjConstructors() starlark.StringDict {
	result := make(starlark.StringDict, len(allSchemas)+1)
	for name, schema := range allSchemas {
		result[name] = makeObjConstructor(name, schema)
	}
	result["crd"] = starlark.NewBuiltin("k8s.obj.crd", crdConstructor)
	return result
}
