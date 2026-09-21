export type LegalDoc = "terms" | "privacy";

export const LEGAL_VERSION = "2026-03-01";

const operatorName =
  process.env.EXPO_PUBLIC_LEGAL_OPERATOR_NAME?.trim() ||
  "o operador desta instalação";
const operatorDocument =
  process.env.EXPO_PUBLIC_LEGAL_OPERATOR_DOCUMENT?.trim() ||
  "documento não informado";
const supportEmail =
  process.env.EXPO_PUBLIC_LEGAL_SUPPORT_EMAIL?.trim() ||
  "canal de suporte informado pelo operador";
const privacyEmail =
  process.env.EXPO_PUBLIC_LEGAL_PRIVACY_EMAIL?.trim() || supportEmail;

export const legalTitles: Record<LegalDoc, string> = {
  terms: "Termos de Uso",
  privacy: "Política de Privacidade",
};

/** Texto modelo — substituir após revisão jurídica. */
export const legalBodies: Record<LegalDoc, string> = {
  terms: `TERMOS DE USO — LICITALENS (versão ${LEGAL_VERSION})

1. Objeto
O LicitaLens é uma plataforma privada de inteligência comercial que organiza dados públicos de compras governamentais para apoiar a decisão do fornecedor. Não é portal oficial, não envia propostas e não substitui o edital ou assessoria jurídica/contábil.

2. Conta e organização
O cadastro é corporativo (B2B). Você declara poder vincular a organização informada e manter credenciais em sigilo.

3. Planos e cobrança
Quando aplicáveis, assinaturas, períodos de avaliação e limites de uso são descritos no produto e na fatura do provedor de pagamento. Cancelamentos seguem as regras do plano contratado. Instalações self-hosted podem operar sem cobrança integrada.

4. Uso permitido
É proibido revender acesso, extrair dados em massa fora da API contratada, tentar burlar limites ou usar o serviço para fins ilegais.

5. Indicadores e IA
Scores e análises são indicadores de aderência comercial baseados no perfil configurado. Não constituem habilitação, classificação, julgamento ou garantia de resultado em licitação.

6. Dados públicos
Informações de licitações provêm de fontes oficiais públicas; prazos e condições podem mudar. Confirme sempre o edital na fonte indicada.

7. Limitação de responsabilidade
Na extensão permitida pela lei, o LicitaLens não responde por decisões comerciais tomadas apenas com base nos indicadores do produto.

8. Alterações
Podemos atualizar estes termos; a versão vigente será indicada no app.

9. Contato
Dúvidas: ${supportEmail}.`,

  privacy: `POLÍTICA DE PRIVACIDADE — LICITALENS (versão ${LEGAL_VERSION})

1. Controlador
${operatorName}, identificação: ${operatorDocument}, contato de privacidade: ${privacyEmail}.

2. Dados tratados
- Cadastro: nome, e-mail, organização, credenciais (senha armazenada de forma segura).
- Uso: perfil comercial, pipeline, preferências de alerta, tokens de push.
- Cobrança: dados processados pelo Stripe (não armazenamos cartão completo).
- Técnicos: logs, IP, identificadores de sessão para segurança.

3. Finalidades
Prestação do serviço, autenticação, alertas contratados, faturamento, suporte, segurança e melhoria do produto.

4. Base legal (LGPD)
Execução de contrato, legítimo interesse (segurança e melhoria), e consentimento quando aplicável (ex.: canais opcionais de comunicação).

5. Compartilhamento
Provedores de infraestrutura, e-mail, pagamentos e notificações push, sob contratos e medidas de proteção.

6. Retenção
Mantemos dados enquanto a conta estiver ativa e pelo prazo legal posterior ao encerramento, conforme política interna.

7. Direitos do titular
Acesso, correção, exclusão, portabilidade e revogação de consentimento: ${privacyEmail}.

8. Segurança
Medidas técnicas e organizacionais proporcionais ao risco; nenhum sistema é 100% invulnerável.

9. Alterações
Esta política pode ser atualizada; a data da versão aparece no app.`,
};
