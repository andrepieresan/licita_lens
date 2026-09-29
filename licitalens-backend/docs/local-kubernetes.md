# Kubernetes local com dados reais

Este caminho reproduz localmente o modo self-hosted: `DEMO_MODE=false`, PostgreSQL/pgvector, Redpanda, MinIO, ingestão PNCP, procurement, gateway, notificações, migrations e bootstrap do proprietário.

Ele é destinado a demonstração local. As senhas de `values-local.yaml` são apenas credenciais descartáveis de desenvolvimento; não use esse arquivo em produção.

## Requisitos

- Docker;
- Kind;
- kubectl;
- Helm;
- Bun para o app Expo Web;
- K9s (opcional, para observar o cluster).

## Criar imagens locais

Na raiz de `licitalens-backend`:

```bash
kind create cluster --name licitalens

for service in gateway admin ingestion procurement notifications; do
  docker build --build-arg SERVICE="$service" -t "licitalens-${service}:local" .
done

docker build -t licitalens-mobile:local ../licitalens-mobile

kind load docker-image \
  licitalens-gateway:local \
  licitalens-admin:local \
  licitalens-ingestion:local \
  licitalens-procurement:local \
  licitalens-notifications:local \
  licitalens-mobile:local \
  --name licitalens
```

## Instalar a stack

```bash
helm upgrade --install licitalens ./deploy/helm/licitalens \
  --namespace licitalens \
  --create-namespace \
  -f ./deploy/helm/licitalens/values-local.yaml \
  --timeout 10m
```

O chart cria os componentes e executa os jobs em ordem: migration, tópicos Redpanda e bootstrap da conta. Cada job aguarda sua dependência ficar disponível. A consulta ao PNCP roda em um `CronJob` do Kubernetes a cada 30 minutos, usando somente as APIs oficiais configuradas para o PNCP. Cada execução grava resultado e erro no PostgreSQL.

Verifique:

```bash
kubectl -n licitalens get pods,jobs,cronjobs
kubectl -n licitalens port-forward svc/licitalens-licitalens-gateway 8080:80
curl http://localhost:8080/health/ready
```

## Visualizar com K9s

O K9s roda no computador e apenas observa o cluster; ele não é um serviço do LicitaLens nem precisa ser instalado no Helm. O contexto local é `kind-licitalens`.

Depois de criar o cluster e instalar o chart:

```bash
k9s --context kind-licitalens -n licitalens
```

Dentro do K9s, use `:pod` para ver os pods, `l` para logs, `d` para detalhes, `:job` para acompanhar execuções e `:cronjob` para ver o agendamento PNCP.

No macOS, se ainda não estiver instalado:

```bash
brew install derailed/k9s/k9s
```

## Abrir o app real

O Expo Web e o servidor Expo Go rodam no pod `licitalens-mobile`. Encaminhe as portas do gateway e do app:

```bash
kubectl -n licitalens port-forward --address 0.0.0.0 svc/licitalens-licitalens-gateway 8080:80
kubectl -n licitalens port-forward --address 0.0.0.0 svc/licitalens-licitalens-mobile 19006:19006
kubectl -n licitalens port-forward --address 0.0.0.0 svc/licitalens-licitalens-expo-go 8082:8082
```

Abra `http://localhost:19006` para web. No Expo Go, informe `exp://<IP-DO-COMPUTADOR>:8082`. Defina `mobile.host` e `mobile.apiUrl` em `values-local.yaml` com o IP atual do computador antes de instalar ou atualizar o chart. Entre com `owner@example.com` e `local-validation-password`, complete o perfil e consulte Radar/Licitações.

## Diagnóstico

```bash
kubectl -n licitalens get jobs --sort-by=.metadata.creationTimestamp
kubectl -n licitalens logs job/<nome-do-job-pncp>
kubectl -n licitalens logs deploy/licitalens-licitalens-procurement -f
kubectl -n licitalens logs deploy/licitalens-licitalens-mobile -f
```

Para remover somente este ambiente local:

```bash
helm uninstall licitalens -n licitalens
kubectl delete namespace licitalens
kind delete cluster --name licitalens
```
