# Runbook

## Ingestão atrasada

1. Verificar `/health/ready`, lag do consumer e último checkpoint.
2. Confirmar disponibilidade da fonte sem registrar URLs sensíveis.
3. Reiniciar somente o consumidor afetado.
4. Executar `licitalens sync --max-pages=1` apenas como diagnóstico local; ele não alimenta o PostgreSQL.
5. O worker recupera automaticamente os dias posteriores ao cursor `pncp/recovery` após reinício. Confirme o avanço do cursor e as métricas de atraso; para uma interrupção maior que `PNCP_MAX_RECOVERY_DAYS`, aumente esse valor ou use `0` para recuperação sem limite.

## Dead-letter queue

1. Identificar a versão do evento e a causa permanente.
2. Corrigir o consumidor e implantar uma versão compatível.
3. Inspecionar `procurement.dead-letter.v1` com `rpk topic consume`, corrigir o payload e republicá-lo manualmente em `procurement.discovered.v1` com a chave da oportunidade original.
4. Comparar contagens e offsets antes de remover mensagens retidas. O CLI também oferece `licitalens replay-dlq` para republicação controlada quando a operação estiver configurada com o broker correto.

## IA indisponível

- Manter feed, filtros e score em operação.
- Marcar explicações novas como pendentes e não consumir cota em falhas.
- Trocar `AI_PROVIDER` para o adaptador saudável somente após validar a mesma dimensão de embeddings.
