{{- define "nexus.name" -}}{{ .Release.Name | trunc 50 | trimSuffix "-" }}{{- end -}}

{{- define "nexus.labels" -}}
app.kubernetes.io/name: nexus-mirror
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
kubetre.io/service: {{ .Values.kubetre.service | quote }}
{{- end -}}

{{- define "nexus.podSecurity" -}}
runAsNonRoot: true
runAsUser: 200
runAsGroup: 200
fsGroup: 200
seccompProfile: {type: RuntimeDefault}
{{- end -}}

{{- define "nexus.containerSecurity" -}}
allowPrivilegeEscalation: false
capabilities: {drop: ["ALL"]}
{{- end -}}

{{- /* name|format|json-body records separated by ';' for the setup job. */ -}}
{{- define "nexus.repositories" -}}
{{- $out := list -}}
{{- range .Values.repositories -}}
{{- $body := dict "name" .name "online" true
    "storage" (dict "blobStoreName" "default" "strictContentTypeValidation" true)
    "proxy" (dict "remoteUrl" .remoteUrl "contentMaxAge" 1440 "metadataMaxAge" 1440)
    "negativeCache" (dict "enabled" true "timeToLive" 1440)
    "httpClient" (dict "blocked" false "autoBlock" true) -}}
{{- $body = mergeOverwrite $body (.extra | default dict) -}}
{{- $out = append $out (printf "%s|%s|%s" .name .format (toJson $body)) -}}
{{- end -}}
{{- join ";" $out -}}
{{- end -}}
