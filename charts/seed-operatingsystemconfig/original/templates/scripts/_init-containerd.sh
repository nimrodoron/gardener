{{- define "init-containerd" -}}
- path: /opt/bin/init-containerd
  permissions: 0755
  content:
    inline:
      encoding: ""
      data: |
        #!/bin/bash

        mkdir -p /etc/containerd
        containerd config default > /etc/containerd/config.toml
{{- end -}}
q

:q

: