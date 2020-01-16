# Documentation Index

## Overview

* [General Architecture](https://github.com/gardener/documentation/wiki/Architecture)
* [Gardener landing page `gardener.cloud`](https://gardener.cloud/)
* ["Gardener, the Kubernetes Botanist" blog on kubernetes.io](https://kubernetes.io/blog/2018/05/17/gardener/)

## Concepts

* [Gardener API server](concepts/apiserver.md)
* [Gardener Scheduler](concepts/scheduler.md)
* [Gardenlet](concepts/gardenlet.md)

## Usage

* [Audit a Kubernetes cluster](usage/shoot_auditpolicy.md)
* [Custom `CoreDNS` configuration](usage/custom-dns.md)
* [Trusted TLS certificate for shoot control planes](usage/trusted-tls-for-control-planes.md)
* [Gardener configuration and usage](usage/configuration.md)
* [OpenIDConnect presets](usage/openidconnect-presets.md)
* [Supported Kubernetes versions](usage/supported_k8s_versions.md)
* [Trigger shoot operations](usage/shoot_operations.md)
* [Troubleshooting guide](usage/trouble_shooting_guide.md)

## Proposals

* [GEP-1: Gardener extensibility and extraction of cloud-specific/OS-specific knowledge](proposals/01-extensibility.md)
* [GEP-2: `BackupInfrastructure` CRD and Controller Redesign](proposals/02-backupinfra.md)
* [GEP-3: Network extensibility](proposals/03-networking-extensibility.md)
* [GEP-4: New `core.gardener.cloud/v1alpha1` APIs required to extract cloud-specific/OS-specific knowledge out of Gardener core](proposals/04-new-core-gardener-cloud-apis.md)
* [GEP-5: Gardener Versioning Policy](proposals/05-versioning-policy.md)
* [GEP-6: Integrating etcd-druid with Gardener](proposals/06-etcd-druid.md)
* [GEP-7: Shoot Control Plane Migration](proposals/07-shoot-control-plane-migration.md)
* [GEP-8: SNI Passthrough proxy for kube-apiservers](proposals/08-shoot-apiserver-via-sni.md)
* [GEP-9: Gardener integration test framework](proposals/09-test-framework.md)

## Development

* [Setting up a local development environment](development/local_setup.md)
* [Unit Testing and Dependency Management](development/testing_and_dependencies.md)
* [Features, Releases and Hotfixes](development/process.md)
* [Adding New Cloud Providers](development/new-cloud-provider.md)
* [Extending the Monitoring Stack](development/monitoring-stack.md)
* [How to create log parser for container into fluent-bit](development/log_parsers.md)
* [User Alerts](development/user_alerts.md)
* [Operator Alerts](development/operator_alerts.md)

## Extensions

* [Extensibility overview](extensions/overview.md)
* [Extension controller registration](extensions/controllerregistration.md)
* [`Cluster` resource](extensions/cluster.md)
* Extension points
  * [General conventions](extensions/conventions.md)
  * [Trigger for reconcile operations](extensions/reconcile-trigger.md)
  * [Deploy resources into the shoot cluster](extensions/managedresources.md)
  * [Shoot resource customization webhooks](extensions/shoot-webhooks.md)
  * [Logging and Monitoring configuration](extensions/logging-and-monitoring.md)
  * [Contributing to shoot health status conditions](extensions/shoot-health-status-conditions.md)
  * Blob storage providers
    * [`BackupBucket` resource](extensions/backupbucket.md)
    * [`BackupEntry` resource](extensions/backupentry.md)
  * DNS providers
    * [`DNSProvider` and `DNSEntry` resources](extensions/dns.md)
  * IaaS/Cloud providers
    * [Control plane customization webhooks](extensions/controlplane-webhooks.md)
    * [`ControlPlane` resource](extensions/controlplane.md)
    * [`ControlPlane` exposure resource](extensions/controlplane-exposure.md)
    * [`Infrastructure` resource](extensions/infrastructure.md)
    * [`Worker` resource](extensions/worker.md)
  * Network plugin providers
    * [`Network` resource](extensions/network.md)
  * Operating systems
    * [`OperatingSystemConfig` resource](extensions/operatingsystemconfig.md)
  * Generic (non-essential) extensions
    * [`Extension` resource](extensions/extension.md)

## Testing

* [Integration Testing Manual](testing/integration_tests.md)

## Deployment

* [Deploying the Gardener into a Kubernetes cluster](deployment/kubernetes.md)
* [Deploying the Gardener and a Seed into an AKS cluster](deployment/aks.md)
* [Overwrite image vector](deployment/image_vector.md)

## Monitoring

* [Alerting](monitoring/alerting.md)
