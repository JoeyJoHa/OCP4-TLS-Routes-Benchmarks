# TLS benchmark guides

| Guide | Use when |
| --- | --- |
| [Running locally](running-locally.md) | Go installed; fastest laptop loop |
| [Running with Podman/Docker](running-with-podman-docker.md) | Same container image as OpenShift, before you deploy |
| [Running on OpenShift](running-on-openshift.md) | PVC, Service, Routes, TLS Secrets |
| [TLS certificates](tls-certificates.md) | RSA / ECDSA profiles; volume vs Secret |
| [TLS benchmark methodology](tls-benchmark-methodology.md) | Handshake vs bulk matrices, percentiles, Route labels |

| | Local (Go) | Podman/Docker | OpenShift |
| --- | --- | --- | --- |
| Process | `bin/` from `make run` | `tlsbench:dev` | pushed image + manifests |
| Storage | `./data` | compose volume | PVC |
| TLS certs | auto or `./certs` | volume mount | Secret or auto-generated |
| Routes | N/A | N/A | edge / passthrough / reencrypt |
| Client | `curl` localhost | `curl` localhost | Linux VM → Route |

Start with **Podman** if you do not have Go. Use [methodology](tls-benchmark-methodology.md) for comparison tables, not for first-time install.
