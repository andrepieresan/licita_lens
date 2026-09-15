type ApiErrorBody = {
  message?: string;
  code?: string;
  error?: string;
};

export function messageFromApiBody(body: unknown, status: number): string {
  if (body && typeof body === "object") {
    const record = body as ApiErrorBody;
    if (typeof record.message === "string" && record.message.trim()) {
      return record.message.trim();
    }
    if (typeof record.error === "string" && record.error.trim()) {
      return record.error.trim();
    }
  }
  if (typeof body === "string" && body.trim()) {
    const trimmed = body.trim();
    if (trimmed.startsWith("{")) {
      try {
        return messageFromApiBody(JSON.parse(trimmed), status);
      } catch {
        /* ignore */
      }
    }
    return trimmed;
  }
  if (status === 404) {
    return "Serviço não encontrado. Verifique se a API está atualizada (make commercial-up).";
  }
  if (status === 401) {
    return "Sessão expirada. Entre novamente.";
  }
  if (status === 403) {
    return "Sem permissão para esta ação.";
  }
  if (status === 402) {
    return "Assinatura inativa. Ative o trial ou um plano.";
  }
  if (status === 429) {
    return "Limite de uso atingido. Tente mais tarde ou faça upgrade.";
  }
  return `Falha na API (HTTP ${status}).`;
}

export function userFacingError(cause: unknown, fallback = "Não foi possível concluir a operação."): string {
  if (cause instanceof Error && cause.message.trim()) {
    return cause.message;
  }
  if (typeof cause === "string" && cause.trim()) {
    return cause.trim();
  }
  return fallback;
}
