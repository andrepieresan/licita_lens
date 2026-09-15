export type SessionPayload = {
  access_token: string;
  account: { id: string; email: string; full_name: string };
  organization: { id: string; name: string; status: string };
  subscription: { plan: string; status: string; external_id?: string; updated_at?: string };
};

export type DealStage = "prospecting" | "analysis" | "proposal" | "negotiation" | "won" | "lost";

export type DealFollowUp = {
  id: string;
  deal_id: string;
  note: string;
  scheduled_at?: string;
  created_at: string;
};

export type NotificationPreferencesPayload = {
  push: boolean;
  email: boolean;
  whatsapp: boolean;
  deadline_reminder: boolean;
};

export type Deal = {
  id: string;
  organization_id: string;
  opportunity_id?: string;
  title: string;
  buyer_name: string;
  stage: DealStage;
  estimated_value_cents: number;
  next_follow_up_at?: string;
  closed_at?: string;
  last_follow_up_note?: string;
  created_at?: string;
  updated_at?: string;
};

export const DEAL_STAGES: Array<{ key: DealStage; label: string }> = [
  { key: "prospecting", label: "Prospecção" },
  { key: "analysis", label: "Análise" },
  { key: "proposal", label: "Proposta" },
  { key: "negotiation", label: "Negociação" },
  { key: "won", label: "Fechado" },
  { key: "lost", label: "Perdido" },
];
