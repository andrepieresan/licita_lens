# Modelo de ameaça inicial

## Ativos

- Identidade, associação a organizações e papéis.
- Perfil comercial, preferências e destinos de alertas.
- Chaves de provedores, eventos Stripe e histórico de consumo.
- Dados públicos enriquecidos e resultados privados de matching.

## Controles obrigatórios

- Keycloak valida usuário por OIDC; o gateway remove headers internos enviados pelo cliente.
- Toda consulta privada recebe `organization_id` após validação da associação, nunca diretamente de claims não verificados.
- Webhooks exigem assinatura, tolerância curta de timestamp e registro idempotente do identificador externo.
- WhatsApp exige consentimento registrável e opt-out imediato.
- Segredos entram por secret store externo; logs nunca incluem token, documento completo ou payload de webhook.
- NetworkPolicies restringem bancos e serviços internos ao namespace da aplicação.
- Backups são criptografados e restores são testados trimestralmente.

## Limites de IA

- O modelo não determina fraude, elegibilidade jurídica ou decisão de compra.
- Explicações citam evidências persistidas e carregam versão do prompt/modelo.
- Falha, timeout ou resposta inválida não altera o score determinístico.

