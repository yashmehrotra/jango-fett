# Jango Fett Design Principles

The purpose to create Jango Fett was to ~~be used as the clone template of the Grand Army of the Galactic Republic~~ give developers their own fungible environments.

It works via creating an `Environment` CRD which is responsible for creating the namespace, config and RBACs. Once you have created a namespace, you would need to create datasources for which we have `PostgreSQL` and `Elasticsearch` CRD.

When writing reconcilers, the end goal should be achieving a desired state. To enforce this philosophy, the reconciler function only receives the name and namespace of the object and it is upto us to apply the changes or delete the resources if it does not exist.


### Environment

Environment's Spec has 2 attributes:

- `config`: A config map `jango-fett` will be created in the same namespace with whatever key-value pairs are inside config. It can be used to provide configuration to any workloads that run in the namespace.
- `users`: This is the list of users who will have access to the environment. The first item in the list will be considered the owner of the namespace.

When the CRD is applied, the namespace created will have the name `jf-<metadata.name>`

Sample:

```yaml
apiVersion: jango-fett.bukukas.io/v1alpha1
kind: Environment
metadata:
  name: sample
  namespace: yash
spec:
  users:
    - yash@beecash.io
    - madhukar@beecash.io
  config:
    foo: bar
    enable: yes
```


### PostgreSQL

When the CRD is applied, the controller's job is to create a PostgreSQL Database and delete it when the CRD is deleted. It creates a new user and password and stores the credentials in a secret named `jf-postgresql-<metadata.name>`.

Sample:

```yaml
apiVersion: jango-fett.bukukas.io/v1alpha1
kind: PostgreSQL
metadata:
  name: db-name
  namespace: jf-sample
spec: {}
```

This will create a database named `jf_db_name` with a user named `jf_db_name` and a random password.

### Elasticsearch

When the CRD is applied, the controller's job is to create an Elasticsearch index and delete it when the CRD is deleted. It creates a new user and password and stores the credentials in a secret named `jf-elasticsearch-<metadata.name>`.

Sample:

```yaml
apiVersion: jango-fett.bukukas.io/v1alpha1
kind: Elasticsearch
metadata:
  name: index-name
  namespace: jf-sample
spec: {}
```

This will create a database named `jf_index_name` with a user named `jf_index_name` and a random password.
 
