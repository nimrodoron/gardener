{{- define "init-containerd-script" -}}
- path: /opt/bin/init-containerd
  permissions: 0755
  content:
    inline:
      encoding: ""
      data: |
        #!/bin/bash

        FILE=/etc/containerd/config.toml
        if [ ! -f "$FILE" ]; then
          mkdir -p /etc/containerd
          containerd config default > "$FILE"
        fi
{{- end -}}