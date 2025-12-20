# LibOps Control Plane

LibOps is an event-driven infrastructure orchestration system that manages GCP resources and VM configurations across organizations, projects, and sites. It uses event aggregation, debouncing, and fan-out patterns to efficiently reconcile infrastructure state changes.

## Architecture

The system consists of several core components:

*   **API**: The central management API (Go/ConnectRPC) serving the dashboard and handling API requests.
*   **Event Router**: Polls the event queue and orchestrates reconciliations using `go-workflows`.
*   **Site Proxy**: A Cloud Run service that fans out events to individual site controllers.
*   **Controller**: Runs on site VMs to execute reconciliations (SSH keys, secrets, firewall, deployments).
*   **Databases**: MariaDB (application data) and PostgreSQL (workflow state).
*   **Security**: HashiCorp Vault for secret management.
