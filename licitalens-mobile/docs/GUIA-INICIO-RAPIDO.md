# LicitaLens — Guia de início rápido

Use este roteiro na **primeira vez** no app (cadastro → radar → pipeline → alertas). Tempo estimado: **15–20 minutos**.

**Dentro do app:** na boas-vindas, **Tour guiado (cadastro e uso)**; logado, **Conta → Abrir tour guiado** (modal com passos e atalhos para cada aba).

---

## Antes de abrir o app (ambiente local)

| O quê | Comando / URL |
|--------|----------------|
| API + **dados reais PNCP** (radar) | `cd licitalens-backend && make phase1-up` |
| API + banco + alertas (com seed demo) | `cd licitalens-backend && make commercial-up` |
| App (modo SaaS, **não** demo offline) | `cd licitalens-mobile && bun run commercial:web` |
| Abrir no navegador | URL que o Expo mostrar (ex.: http://localhost:8081) |

> **Importante:** não use `bun run demo:web` para este guia — o demo roda sem login real. Use `commercial:web`.

---

## Mapa do fluxo (visão geral)

```
┌─────────────────────────────────────────────────────────────────┐
│  BOAS-VINDAS                                                     │
│  [ Criar conta ]     [ Já tenho conta ]                          │
└────────────┬───────────────────────────────┬────────────────────┘
             │                               │
             ▼                               ▼
      ┌──────────────┐                ┌──────────────┐
      │   CADASTRO   │                │    LOGIN     │
      └──────┬───────┘                └──────┬───────┘
             │                               │
             └───────────────┬───────────────┘
                             ▼
              ┌──────────────────────────┐
              │ ATIVAÇÃO (se necessário)  │
              │ Trial ou Stripe Pro       │
              └────────────┬─────────────┘
                           ▼
              ┌──────────────────────────┐
              │ ONBOARDING — Perfil       │
              │ (o que sua empresa vende) │
              └────────────┬─────────────┘
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│  APP — barra inferior:                                           │
│  [ Pipeline ] [ Licitações ] [ Perfil ] [ Plano ] [ Conta ]      │
└──────────────────────────────────────────────────────────────────┘
```

---

## Parte 1 — Criar conta e entrar

### 1. Boas-vindas

- Leia o texto introdutório e toque em **Criar conta** (conta nova) ou **Já tenho conta** (quem já se cadastrou).

### 2. Cadastro (conta nova)

Preencha:

| Campo | Dica |
|--------|------|
| Nome completo | Seu nome |
| E-mail corporativo | Use um e-mail que você consiga acessar (alertas usam este endereço) |
| Senha | Mínimo 8 caracteres |
| Nome da empresa | Nome da organização no LicitaLens |

Toque **Criar conta e continuar**. Se algo falhar, a mensagem aparece em vermelho abaixo dos botões.

### 3. Ativação da assinatura

Se aparecer **Ative sua assinatura**:

- **Continuar com trial** — caminho usual para testar (recomendado no primeiro uso).
- **Assinar Pro (Stripe)** — só funciona se o backend tiver Stripe configurado.
- **Voltar ao login** — encerra a sessão e volta ao início.

### 4. Onboarding — perfil comercial

- **Nome do perfil** — ex.: “Radar principal”.
- **Descrição** — descreva o que a empresa fornece (ex.: “notebooks e suporte de TI para órgãos públicos”).

Toque **Ir para o painel**. Você entra no app com a barra inferior visível.

### 5. Login (quem já tem conta)

- E-mail e senha → **Entrar**.
- O app lembra a sessão ao recarregar a página.

---

## Parte 2 — Conhecer as cinco abas

| Aba | Nome no app | Para quê serve |
|-----|-------------|----------------|
| 1 | **Pipeline** | Kanban/lista dos negócios em andamento |
| 2 | **Licitações** | Radar de oportunidades públicas |
| 3 | **Perfil** | Ajustar palavras-chave, descrição e estados (UF) |
| 4 | **Plano** | Uso do plano, checkout Stripe, histórico |
| 5 | **Conta** | Alertas (e-mail/push), sair |

No topo aparecem o **nome da empresa** e o **seu primeiro nome**.

---

## Parte 3 — Roteiro “primeiro negócio” (recomendado)

Siga esta ordem uma vez; depois você repete só **Licitações → Pipeline**.

### Passo A — Afinar o perfil (2 min)

1. Aba **Perfil**.
2. Preencha:
   - **O que sua empresa fornece** (texto livre).
   - **Palavras-chave** separadas por vírgula (ex.: `notebooks, informática`).
   - **Estados** com siglas (ex.: `PR, SC`).
3. **Salvar perfil**.

> Quanto melhor o perfil, melhor o match nas licitações e nos alertas.

### Passo B — Explorar licitações (3 min)

1. Aba **Licitações**.
2. Puxe a lista para baixo para **atualizar** (refresh).
3. Toque em uma licitação para abrir o detalhe.
4. No modal:
   - Veja **valor** e **prazo**.
   - **Analisar aderência** — score e explicação em relação ao seu perfil.
   - **Adicionar ao pipeline** — cria um card ligado à licitação.
   - **Abrir fonte oficial** — link externo (PNCP/fonte).

Após adicionar ao pipeline, o app leva você à aba **Pipeline**.

### Passo C — Trabalhar o pipeline (5 min)

1. Aba **Pipeline** (se não estiver já nela).
2. Alterne **Kanban** / **Lista** no topo, se disponível.
3. Em um card:
   - **Avançar etapa** — move no funil: Prospecção → Análise → Proposta → Negociação → Fechado.
   - **Ver licitação** — reabre a oportunidade no radar (se o card veio de uma licitação).
   - **Salvar follow-up** — registre uma nota de contato comercial.
   - **Marcar perdido** — encerra como perdido.

4. Card manual: use o campo de título + **Adicionar** para negócios que não vieram do radar.

### Passo D — Alertas na Conta (2 min)

1. Aba **Conta**.
2. Seção **ALERTAS** — ligue/desligue:

| Switch | Efeito |
|--------|--------|
| Notificações no app | Push no celular (requer app nativo + permissão; na web não registra push) |
| Resumo por e-mail | Worker envia e-mails de novas oportências compatíveis |
| WhatsApp | Preferência guardada; canal ainda experimental |
| Lembrete de prazo | Lembretes de follow-up comercial por e-mail |

As preferências **sincronizam com o servidor** quando a API está no ar.

3. **Sair da conta** — encerra a sessão neste dispositivo.

---

## Parte 4 — Uso do dia a dia (rotina simples)

```
  Manhã / início do dia
        │
        ▼
  Licitações ──► analisar novas ──► adicionar ao pipeline
        │
        ▼
  Pipeline ──► mover etapas ──► registrar follow-ups
        │
        ▼
  Perfil (se mudou portfólio ou região)
        │
        ▼
  Conta (se quiser mudar e-mail/push)
```

---

## Etapas do pipeline (referência)

| Etapa no app | Significado prático |
|--------------|---------------------|
| Prospecção | Lead identificado, ainda não analisado a fundo |
| Análise | Avaliando viabilidade e documentos |
| Proposta | Elaborando ou enviando proposta |
| Negociação | Contato ativo com o órgão |
| Fechado | Ganho |
| Perdido | Descartado |

---

## Problemas comuns

| Situação | O que fazer |
|----------|-------------|
| “Falha ao carregar dados” | Confirme `make commercial-up` e http://localhost:8080/health/ready |
| Lista de licitações vazia | Atualize a aba Licitações; confira se o backend tem seed/dados |
| Preferências não sincronizaram | Mensagem amarela/erro — API offline ou sessão expirada; entre de novo |
| Checkout Stripe não abre | Stripe não configurado no backend — use trial |
| Push não chega | Use app em **celular físico**, push ligado em Conta; e-mail testável via Mailpit (dev) |

---

## Checklist — “concluí o básico”

- [ ] Criei conta ou fiz login
- [ ] Passei pelo onboarding / salvei perfil com keywords e UF
- [ ] Analisei uma licitação e vi o score
- [ ] Adicionei ao pipeline e movi de etapa
- [ ] Registrei um follow-up
- [ ] Ajustei alertas em Conta
- [ ] Sei onde fica Plano e como sair da conta

---

*Versão alinhada ao fluxo `SaasFlow` (Expo). Modo demo offline: `App.tsx` com `EXPO_PUBLIC_DEMO_MODE=true` — fluxo parecido, sem API.*
