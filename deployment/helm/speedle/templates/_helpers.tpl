{{/*
Expand the chart name.
*/}}
{{- define "speedle.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Full name, truncated for DNS labels.
*/}}
{{- define "speedle.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "speedle.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "speedle.componentName" -}}
{{- /* DNS labels are 63 characters. Reserve the separator and the component. */ -}}
{{- $base := include "speedle.fullname" .root | trunc 59 | trimSuffix "-" -}}
{{- printf "%s-%s" $base .component -}}
{{- end -}}

{{- define "speedle.policyClaim" -}}
{{- if .Values.store.file.persistence.existingClaim -}}
{{- .Values.store.file.persistence.existingClaim -}}
{{- else -}}
{{- $base := include "speedle.fullname" . | trunc 54 | trimSuffix "-" -}}
{{- printf "%s-policies" $base -}}
{{- end -}}
{{- end -}}

{{- define "speedle.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "speedle.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "speedle.image" -}}
{{- $tag := .Values.image.tag -}}
{{- if not $tag -}}
{{- $tag = .Chart.AppVersion -}}
{{- end -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{- define "speedle.selectorLabels" -}}
app.kubernetes.io/name: {{ include "speedle.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "speedle.labels" -}}
helm.sh/chart: {{ include "speedle.chart" .root }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
{{ include "speedle.selectorLabels" . }}
{{- end -}}

{{- define "speedle.probeTiming" -}}
periodSeconds: {{ .periodSeconds }}
timeoutSeconds: {{ .timeoutSeconds }}
failureThreshold: {{ .failureThreshold }}
{{- end -}}

{{- define "speedle.httpProbe" -}}
httpGet:
  path: {{ .path }}
  port: http
  scheme: {{ .scheme }}
{{- include "speedle.probeTiming" .timing | nindent 0 }}
{{- end -}}

{{/*
Flags for --store-type and the selected backend.
*/}}
{{- define "speedle.storeArgs" -}}
{{- if eq .Values.store.type "file" -}}
{{- if ne (int .Values.pms.replicaCount) 1 -}}
{{- fail "the file store supports one PMS replica; use etcd or mongodb to run more than one PMS pod" -}}
{{- end -}}
{{- if not .Values.store.file.persistence.enabled -}}
{{- fail "the file store needs store.file.persistence.enabled so PMS and ADS share one volume" -}}
{{- end -}}
{{- if not (hasPrefix "/" .Values.store.file.location) -}}
{{- fail "store.file.location must be an absolute path on the shared volume" -}}
{{- end -}}
{{- $accessMode := .Values.store.file.persistence.accessMode -}}
{{- if not (or (eq $accessMode "ReadWriteOnce") (eq $accessMode "ReadWriteMany")) -}}
{{- fail "store.file.persistence.accessMode must be ReadWriteOnce or ReadWriteMany; PMS writes the policy file" -}}
{{- end -}}
- --store-type
- file
- --filestore-loc
- {{ .Values.store.file.location | quote }}
{{- else if eq .Values.store.type "etcd" -}}
- --store-type
- etcd
- --etcdstore-endpoint
- {{ required "store.etcd.endpoint is required when store.type is etcd" .Values.store.etcd.endpoint | quote }}
{{- if .Values.store.etcd.keyPrefix }}
- --etcdstore-keyprefix
- {{ .Values.store.etcd.keyPrefix | quote }}
{{- end }}
{{- $etcdTLS := .Values.store.etcd.tls -}}
{{- if or $etcdTLS.certFile $etcdTLS.keyFile $etcdTLS.caFile $etcdTLS.crlFile -}}
{{- if not $etcdTLS.secretName -}}
{{- fail "store.etcd.tls.secretName is required when etcd TLS files are set" -}}
{{- end -}}
{{- end -}}
{{- if $etcdTLS.certFile }}
- --etcdstore-tls-cert
- {{ printf "/etc/speedle/etcd/%s" $etcdTLS.certFile | quote }}
{{- end }}
{{- if $etcdTLS.keyFile }}
- --etcdstore-tls-key
- {{ printf "/etc/speedle/etcd/%s" $etcdTLS.keyFile | quote }}
{{- end }}
{{- if $etcdTLS.caFile }}
- --etcdstore-tls-ca
- {{ printf "/etc/speedle/etcd/%s" $etcdTLS.caFile | quote }}
{{- end }}
{{- if $etcdTLS.crlFile }}
- --etcdstore-tls-crlFile
- {{ printf "/etc/speedle/etcd/%s" $etcdTLS.crlFile | quote }}
{{- end }}
{{- if $etcdTLS.allowedCN }}
- --etcdstore-tls-allowedCN
- {{ $etcdTLS.allowedCN | quote }}
{{- end }}
{{- if $etcdTLS.serverName }}
- --etcdstore-tls-serverName
- {{ $etcdTLS.serverName | quote }}
{{- end }}
{{- if $etcdTLS.insecureSkipVerify }}
- --etcdstore-tls-insecureSkipVerify=true
{{- end }}
{{- else if eq .Values.store.type "mongodb" -}}
- --store-type
- mongodb
{{- if .Values.store.mongodb.existingSecret }}
{{- if not .Values.store.mongodb.secretKey }}
{{- fail "store.mongodb.secretKey is required when store.mongodb.existingSecret is set" }}
{{- end }}
{{- if .Values.store.mongodb.uri }}
{{- fail "set either store.mongodb.uri or store.mongodb.existingSecret, not both" }}
{{- end }}
{{- else }}
{{- $mongoURI := required "store.mongodb.uri or store.mongodb.existingSecret is required when store.type is mongodb" .Values.store.mongodb.uri }}
{{- if regexMatch "://[^/?#]*@" $mongoURI }}
{{- fail "store.mongodb.uri must not include credentials; put the URI in a Secret and set store.mongodb.existingSecret" }}
{{- end }}
- --mongostore_uri
- {{ $mongoURI | quote }}
{{- end }}
- --mongostore_database
- {{ required "store.mongodb.database is required when store.type is mongodb" .Values.store.mongodb.database | quote }}
{{- else -}}
{{- fail (printf "store.type must be file, etcd or mongodb, not %q" .Values.store.type) -}}
{{- end -}}
{{- end -}}

{{- define "speedle.tlsArgs" -}}
{{- if .Values.tls.enabled -}}
{{- if not .Values.tls.secretName -}}
{{- fail "tls.secretName is required when tls.enabled is true" -}}
{{- end -}}
- --insecure=false
- --cert
- {{ printf "/etc/speedle/tls/%s" (required "tls.certFile is required when tls.enabled is true" .Values.tls.certFile) | quote }}
- --key
- {{ printf "/etc/speedle/tls/%s" (required "tls.keyFile is required when tls.enabled is true" .Values.tls.keyFile) | quote }}
{{- if .Values.tls.clientCAFile }}
- --client-cert
- {{ printf "/etc/speedle/tls/%s" .Values.tls.clientCAFile | quote }}
{{- end }}
{{- if .Values.tls.forceClientCert }}
- --force-client-cert=true
{{- end }}
{{- else -}}
- --insecure=true
{{- end -}}
{{- end -}}
