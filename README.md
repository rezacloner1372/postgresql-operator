# PostgreSQL Operator

This repository contains a Kubernetes operator for managing PostgreSQL instances using the Operator SDK. The operator automates the deployment and management of PostgreSQL in a Kubernetes environment.

## Table of Contents
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Deploying the Operator](#deploying-the-operator)
- [Testing the Operator](#testing-the-operator)
- [Cleaning Up](#cleaning-up)
- [License](#license)

## Prerequisites

Before you begin, ensure you have the following installed:

- [Docker](https://docs.docker.com/get-docker/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- [KIND](https://kind.sigs.k8s.io/docs/user/quick-start/)
- [Operator SDK](https://sdk.operatorframework.io/docs/install-operator-sdk/)
- [Go](https://golang.org/dl/) (if you need to build or modify the operator)

## Installation

1. **Clone the Repository**
   ```bash
   git clone https://github.com/your-username/postgres-operator.git
   cd postgres-operator
   ```

2. **Build the Operator**
   Ensure you're in the operator's root directory, then build the operator:
   ```bash
   make docker-build
   ```

3. **Push the Operator Image to Local Registry**
   If you're using a local Docker registry, tag and push the image:
   ```bash
   make docker-push
   ```

   If you don't have a local registry set up, you can simply use the KIND-internal registry. Run the following command to create a KIND cluster with the internal registry:
   ```bash
   kind create cluster --name postgres-cluster --image kindest/node:v1.24.0 --config kind-config.yaml
   ```

   Then, you can load your Docker image into KIND using:
   ```bash
   kind load docker-image your-operator-image:latest --name postgres-cluster
   ```

## Deploying the Operator

1. **Create the Namespace**
   Create a namespace for the PostgreSQL operator:
   ```bash
   kubectl create namespace postgres-operator
   ```

2. **Deploy Custom Resource Definitions (CRDs)**
   Apply the CRDs to your Kubernetes cluster:
   ```bash
   make install
   ```

3. **Deploy the Operator**
   Deploy the operator to the cluster:
   ```bash
   make deploy
   ```

4. **Verify the Operator is Running**
   Check the operator's deployment:
   ```bash
   kubectl get deployment -n postgres-operator
   ```

## Testing the Operator

1. **Create a PostgreSQL Custom Resource**
   Create a YAML file named `postgres.yaml` with the following content:
   ```yaml
    apiVersion: postgres.snappcloud.io/v1alpha1
    kind: Postgres
    metadata:
    labels:
        app.kubernetes.io/name: postgresql-operator
        app.kubernetes.io/managed-by: kustomize
    name: mypostgres
    spec:
    version: "13" # Example: "13"
    persistence:
        size: "1Gi" # Example: "1Gi"
    auth:
        database: "postgres" # Example: "postgres"
        secretRef: "credentials" # Reference to pre-existing secret containing
    status:
        ready: flase # Indicates readiness status
   ```

2. **Create a Secret for Database Credentials**
   Create a Kubernetes Secret with the database password:
   ```yaml
  apiVersion: v1
    kind: Secret
    metadata:
    name: credentials
    type: Opaque
    stringData:
    username: postgres
    password: mypassword
   ```
   ```bash
   kubectl apply -f credentials.yaml -n postgres-operator
   ```

3. **Apply the Custom Resource**
   Apply the PostgreSQL custom resource to the cluster:
   ```bash
   kubectl apply -f postgres.yaml -n postgres-operator
   ```

4. **Verify the StatefulSet and Service are Created**
   Check the created StatefulSet and Service:
   ```bash
   kubectl get statefulsets -n postgres-operator
   kubectl get services -n postgres-operator
   ```

5. **Access the PostgreSQL Pod**
   You can access the PostgreSQL pod using:
   ```bash
   kubectl exec -it my-postgres-0 -n postgres-operator -- psql -U postgres
   ```

## Cleaning Up

To clean up the resources created during testing, run:
# I used SetupWithManager function to watch for the resources your operator owns, ensuring that any changes to the StatefulSet or Service trigger reconciliation. To ensure Kubernetes garbage collection works correctly (i.e., deleting the Postgres CR deletes associated resources), set owner references when creating the StatefulSet and Service. Modify the helper functions to include owner references.

``` bash
    // In statefulSetForPostgres
    sts := &appsv1.StatefulSet{
        // ... existing code ...
    }
    // Set Postgres instance as the owner and controller
    if err := ctrl.SetControllerReference(pg, sts, r.Scheme); err != nil {
        logger.Error(err, "Failed to set owner reference on StatefulSet")
        return nil
    }
    return sts

    // In serviceForPostgres
    svc := &corev1.Service{
        // ... existing code ...
    }
    // Set Postgres instance as the owner and controller
    if err := ctrl.SetControllerReference(pg, svc, r.Scheme); err != nil {
        logger.Error(err, "Failed to set owner reference on Service")
        return nil
    }
    return svc
```