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

kind load docker-image \
  licitalens-gateway:local \
  licitalens-admin:local \
  licitalens-ingestion:local \
  licitalens-procurement:local \
  licitalens-notifications:local \
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

O chart cria os componentes e executa os jobs em ordem: migration, tópicos Redpanda e bootstrap da conta. Cada job aguarda sua dependência ficar disponível. A ingestão começa a consultar o PNCP após o gateway e as dependências ficarem disponíveis.

Verifique:

```bash
kubectl -n licitalens get pods,jobs
kubectl -n licitalens port-forward svc/licitalens-licitalens-gateway 8080:80
curl http://localhost:8080/health/ready
```

## Visualizar com K9s

O K9s roda no computador e apenas observa o cluster; ele não é um serviço do LicitaLens nem precisa ser instalado no Helm.

Depois de criar o cluster e instalar o chart:

```bash
k9s -n licitalens
```

Dentro do K9s, use `:pod` para ver os pods, `l` para logs, `d` para detalhes e `:job` para acompanhar migrations, tópicos e bootstrap.

No macOS, se ainda não estiver instalado:

```bash
brew install derailed/k9s/k9s
```

## Abrir o app real

Em outro terminal:

```bash
cd ../licitalens-mobile
EXPO_PUBLIC_API_URL=http://localhost:8080 bun run commercial:web
```

Abra a URL do Expo Web, entre com `owner@example.com` e `local-validation-password`, complete o perfil e consulte Radar/Licitações.

## Diagnóstico

```bash
kubectl -n licitalens logs deploy/licitalens-licitalens-ingestion -f
kubectl -n licitalens logs deploy/licitalens-licitalens-procurement -f
kubectl -n licitalens get job
```

Para remover somente este ambiente local:

```bash
helm uninstall licitalens -n licitalens
kubectl delete namespace licitalens
kind delete cluster --name licitalens
```
