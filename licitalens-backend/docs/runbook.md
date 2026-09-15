# Runbook

## Ingestão atrasada

1. Verificar `/health/ready`, lag do consumer e último checkpoint.
2. Confirmar disponibilidade da fonte sem registrar URLs sensíveis.
3. Reiniciar somente o consumidor afetado.
4. Executar `licitalens sync --max-pages=1` como diagnóstico.
5. Após recuperação, reprocessar a janela perdida e confirmar a meta de dez minutos.

## Dead-letter queue

1. Identificar a versão do evento e a causa permanente.
2. Corrigir o consumidor e implantar uma versão compatível.
3. Executar `licitalens replay-dlq` com filtro de tópico e intervalo.
4. Comparar contagens antes de remover mensagens retidas.

## IA indisponível

- Manter feed, filtros e score em operação.
- Marcar explicações novas como pendentes e não consumir cota em falhas.
- Trocar `AI_PROVIDER` para o adaptador saudável somente após validar a mesma dimensão de embeddings.

