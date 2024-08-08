# Jango Fett

![Jango Fett](.github/jango-fett.jpg)

## Setup
```
go mod download
make
```

## Running tests

```sh
make test
```

## Bootstrapping
```sh
kubebuilder init --domain bukukas.io --repo gitlab.com/beecash/infra/jango-fett
kubebuilder create api --group jango-fett --version v1alpha1 --kind Environment --resource --controller
kubebuilder create api --group jango-fett --version v1alpha1 --kind PostgreSQL --resource --controller
kubebuilder create api --group jango-fett --version v1alpha1 --kind Elasticsearch --resource --controller
```

## Deploying
The env file which has secrets is kept in `config/manager/jango-fett.env`

```sh
make docker-build
make docker-push
cp <path_to_env_file>.env config/manager/jango-fett.env
kustomize build config/default | kubectl apply -f -
```
