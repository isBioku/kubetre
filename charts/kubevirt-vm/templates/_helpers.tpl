{{- define "kvm.name" -}}{{ .Release.Name | trunc 50 | trimSuffix "-" }}{{- end -}}

{{- define "kvm.labels" -}}
app.kubernetes.io/name: kubevirt-vm
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
kubetre.io/workspace: {{ .Values.kubetre.workspace | quote }}
kubetre.io/service: {{ .Values.kubetre.service | quote }}
{{- end -}}

{{- /* The account password, generated once and kept across upgrades. */ -}}
{{- define "kvm.password" -}}
{{- $existing := lookup "v1" "Secret" .Release.Namespace (printf "%s-credentials" (include "kvm.name" .)) -}}
{{- if and $existing $existing.data -}}
{{- index $existing.data "password" | b64dec -}}
{{- else -}}
{{- printf "%s%s%s" (randAlpha 10) (randNumeric 6) (randAlpha 8) -}}
{{- end -}}
{{- end -}}
