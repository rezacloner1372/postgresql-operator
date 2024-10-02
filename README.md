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
   git clone https://github.com/rezacloner1372/postgres-operator.git
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
   Lets apply the secret:
   ```bash
   kubectl apply -f credentials.yaml -n postgres-operator
   ```

3. **Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects**
   ```bash
   make manifests
   ```
4. **Apply the Custom Resource**
   ```bash
   kubectl apply -f postgresql-operator/config/crd/bases/postgres.snappcloud.io_postgres.yaml
   ```
5. **Run controller from your host**
   ```bash
   make run
   ```
6. **Apply Kind Postgres**
   ```bash
   kubectl apply -f postgresql-operator/config/samples/postgres_v1alpha1_postgres.yaml
   ```
7. **Verify the StatefulSet and Service are Created**
   Check the created StatefulSet and Service:
   ```bash
   kubectl get statefulsets -n postgres-operator
   kubectl get services -n postgres-operator
   ```
8. **Access the PostgreSQL Pod**
   You can access the PostgreSQL pod using:
   ```bash
   kubectl exec -it my-postgres-0 -n postgres-operator -- psql -U postgres
   ```

## Clean Up Test(Finalizer)
   Finalizers in Kubernetes are used to delay the deletion of resources until the controller performs specific cleanup tasks. 
   For example, We define a finalizer on our Postgres resource, the Kubernetes API won’t delete the resource immediately when you issue a delete command. Instead, it will mark the resource for deletion (by setting the deletionTimestamp), and the resource will remain in a "terminating" state until the controller handles the cleanup logic (such as deleting related StatefulSets, Services, Secrets, or other resources the Postgres instance owns). Once the controller completes the cleanup, it removes the finalizer from the resource, allowing Kubernetes to complete the deletion.


   To verify that our finalizer logic works correctly,follow a step-by-step approach. 
   Here's how you can ensure that the finalizer performs as expected:

   1. **Check Finalizer is Set on Creation**
   Ensure that when the PostgreSQL custom resource (CR) is created, the finalizer is added to the resource. You can do this by inspecting the *metadata.finalizers* field in the resource.

   After Create a Postgres CR, inspect its metadata using:
   ```bash
   kubectl get postgres <postgres-name> -o yaml
   ```
   Look for the *metadata.finalizers* field. It should have the value you specified (e.g., finalizer.postgres.snappcloud.io).
   
   2. **Simulate Resource Deletion**
   To test the finalizer in action, attempt to delete the Postgres CR and observe whether Kubernetes waits for the finalizer's logic to execute before fully deleting the resource.

   Delete the Postgres CR:

   ```bash
   kubectl delete postgres <postgres-name>
   ```

   When you issue this command, Kubernetes will set the *deletionTimestamp* but won't delete the resource until the finalizer is removed.

   Check the Resource Status:

   ```bash
   kubectl get postgres <postgres-name> -o yaml
   ```

   Look for the *metadata.deletionTimestamp* field. The resource should be in a *"Terminating"* state, and Kubernetes will not remove it until the finalizer logic runs and clears the finalizers field.

   3. **Ensure Cleanup Logic Executes**
   Check that the finalizePostgres function is correctly cleaning up associated resources (e.g., StatefulSet, Service) before the resource is deleted.

   Verify StatefulSet and Service Deletion: After the Postgres resource enters the "Terminating" state, the operator should execute the cleanup logic. Check if the StatefulSet and Service associated with the Postgres CR are deleted by running:

   ```bash
   kubectl get statefulset <postgres-name>
   kubectl get svc postgres-service
   ```

   These should no longer exist if the cleanup was successful.

   4. **Ensure Finalizer is Removed**
   After the cleanup logic in finalizePostgres runs, ensure the finalizer is removed from the Postgres resource.

   Check Finalizer Removal: The finalizer should be removed from the metadata.finalizers field after the cleanup completes. Check this using:
   ```bash
   kubectl get postgres <postgres-name> -o yaml
   ```
   The finalizers field should now be empty, and Kubernetes will proceed to delete the Postgres CR.

   5. **Verify Deletion Completes**
   Finally, check that the Postgres CR itself is fully deleted after the finalizer logic completes.
   Check Postgres CR Deletion: Ensure the Postgres CR is no longer in the cluster:
   ```bash
   kubectl get postgres <postgres-name>
   ```

   You should see "NotFound" if the deletion process was successful.

## SetupWithManager
 I used SetupWithManager function to watch for the resources operator owns, ensuring that any changes to the StatefulSet or Service trigger reconciliation. To ensure Kubernetes garbage collection works correctly (i.e., deleting the Postgres CR deletes associated resources), set owner references when creating the StatefulSet and Service. Modify the helper functions to include owner references.

```bash
    // In statefulSetForPostgres
    sts := &appsv1.StatefulSet{
        // ...code ...
    }
    // Set Postgres instance as the owner and controller
    if err := ctrl.SetControllerReference(pg, sts, r.Scheme); err != nil {
        logger.Error(err, "Failed to set owner reference on StatefulSet")
        return nil
    }
    return sts

    // In serviceForPostgres
    svc := &corev1.Service{
        // ...code ...
    }
    // Set Postgres instance as the owner and controller
    if err := ctrl.SetControllerReference(pg, svc, r.Scheme); err != nil {
        logger.Error(err, "Failed to set owner reference on Service")
        return nil
    }
    return svc
```