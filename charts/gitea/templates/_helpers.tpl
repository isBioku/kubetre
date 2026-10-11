{{- define "gitea.name" -}}{{ .Release.Name | trunc 50 | trimSuffix "-" }}{{- end -}}

{{- define "gitea.labels" -}}
app.kubernetes.io/name: gitea
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
kubetre.io/service: {{ .Values.kubetre.service | quote }}
{{- end -}}

{{- define "gitea.workspace" -}}{{ if eq .Values.mode "workspace" }}true{{ end }}{{- end -}}

{{- define "gitea.podLabels" -}}
{{ include "gitea.labels" . }}
{{- if include "gitea.workspace" . }}
kubetre.io/vm-access: "true"
{{- end }}
{{- end -}}

{{- define "gitea.podSecurity" -}}
runAsNonRoot: true
runAsUser: 1000
runAsGroup: 1000
fsGroup: 1000
seccompProfile: {type: RuntimeDefault}
{{- end -}}

{{- define "gitea.containerSecurity" -}}
allowPrivilegeEscalation: false
capabilities: {drop: ["ALL"]}
{{- end -}}

{{- /* Gitea's settings, as GITEA__section__KEY variables the image turns into app.ini. */ -}}
{{- define "gitea.env" -}}
- {name: GITEA__database__DB_TYPE, value: sqlite3}
- {name: GITEA__database__PATH, value: /var/lib/gitea/data/gitea.db}
- {name: GITEA__server__HTTP_PORT, value: "3000"}
{{- if include "gitea.workspace" . }}
{{- /* The load balancer's address is chosen by Azure, so links follow the address used. */}}
- {name: GITEA__server__PUBLIC_URL_DETECTION, value: auto}
{{- else }}
- {name: GITEA__server__ROOT_URL, value: {{ printf "http://%s/" (.Values.service.loadBalancerIP | default (printf "%s.%s.svc.cluster.local" (include "gitea.name" .) .Release.Namespace)) | quote }}}
{{- end }}
- {name: GITEA__server__DISABLE_SSH, value: "true"}
- {name: GITEA__server__OFFLINE_MODE, value: "true"}
- {name: GITEA__security__INSTALL_LOCK, value: "true"}
{{- if include "gitea.workspace" . }}
{{- /* Only the workspace's VMs reach it, so its members register their own accounts. */}}
- {name: GITEA__service__DISABLE_REGISTRATION, value: "false"}
- {name: GITEA__service__REQUIRE_SIGNIN_VIEW, value: "true"}
- {name: GITEA__service__DEFAULT_ALLOW_CREATE_ORGANIZATION, value: "true"}
- {name: GITEA__repository__DEFAULT_PRIVATE, value: "private"}
- {name: GITEA__migrations__ALLOW_LOCALNETWORKS, value: "true"}
- {name: GITEA__migrations__ALLOWED_DOMAINS, value: {{ .Values.migrationDomains | quote }}}
{{- else }}
- {name: GITEA__service__DISABLE_REGISTRATION, value: "true"}
- {name: GITEA__service__REQUIRE_SIGNIN_VIEW, value: "false"}
- {name: GITEA__repository__DISABLE_STARS, value: "true"}
- {name: GITEA__repository__MAX_CREATION_LIMIT, value: "0"}
- {name: GITEA__mirror__MIN_INTERVAL, value: "10m"}
- {name: GITEA__migrations__ALLOWED_DOMAINS, value: "github.com,*.github.com,gitlab.com"}
{{- end }}
- {name: GITEA__webhook__ALLOWED_HOST_LIST, value: "none"}
{{- end -}}

{{- define "gitea.mirrors" -}}
{{- $out := list -}}
{{- range .Values.mirrors -}}{{- $out = append $out (printf "%s|%s" .name .url) -}}{{- end -}}
{{- join ";" $out -}}
{{- end -}}
