# Fase 4 — Produto e conformidade

## O que já está no app (SaaS)

- Aviso **“Apoio à decisão comercial”** (radar e detalhe da licitação).
- Score rotulado como **aderência ao perfil**, não habilitação/julgamento.
- Selo **“Dados de demonstração”** quando `source=demo`.
- Bloco **fonte e atualização** + link para edital oficial (`source_url`).
- **Termos de Uso** e **Política de Privacidade** (Conta → Legal) e aceite no cadastro.
- Textos em `licitalens-mobile/src/legal/content.ts` — **revisar com assessoria jurídica** antes do lançamento público.

## Checklist LGPD (operacional)

| Item | Responsável | Status |
|------|-------------|--------|
| Base legal e finalidade no cadastro (prestação do serviço B2B) | Jurídico | Template no app |
| Política de privacidade publicada | Jurídico + produto | Tela no app |
| Direitos do titular (acesso, correção, exclusão) — canal `privacidade@` | Suporte | Definir e-mail real |
| Registro de operações de tratamento (ROPA) | DPO/encarregado | Fora do código |
| DPA com subprocessadores (Stripe, hospedagem, e-mail) | Jurídico | Contratos |
| Retenção e exclusão de conta | Engenharia | Política documentada |
| Incidentes — runbook | Operações | [`runbook.md`](runbook.md) + procedimento interno |

## Posicionamento comercial (não substituir edital)

Reforçar em materiais de marketing o que o produto **não** faz (ver [`PRODUCT-ROADMAP.md`](PRODUCT-ROADMAP.md)).

## Pendências de produto (pós-MVP)

- Itens, contratos e fornecedores enriquecidos no detalhe.
- Analytics persistido (ClickHouse) — não usar gráficos locais como fonte oficial.
- Revisão jurídica formal assinada.

Definition of Done global do roadmap ainda exige **Fase 2** (cobrança/auth produção) validada junto com esta fase.
