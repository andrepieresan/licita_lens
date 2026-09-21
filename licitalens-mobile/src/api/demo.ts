import { Analysis, LicitaLensAPI, Opportunity, Profile, ProfileInput, PublicConfig, Usage } from "./client";
import type { Deal, DealFollowUp, DealStage, NotificationPreferencesPayload, SessionPayload } from "./types";

const now = Date.now();
const date = (days: number) => new Date(now + days * 86_400_000).toISOString();

const opportunities: Opportunity[] = [
  { id: "demo-notebooks", source: "demo", source_id: "demo-notebooks", object: "Aquisição de notebooks corporativos com 16 GB de memória e garantia on-site", organization_name: "Município de Curitiba", state: "PR", municipality: "Curitiba", modality_code: 6, estimated_value_cents: 18000000, published_at: date(-1), proposal_deadline: date(9), updated_at: date(0), source_url: "https://pncp.gov.br/" },
  { id: "demo-suporte", source: "demo", source_id: "demo-suporte", object: "Contratação de suporte técnico e manutenção preventiva de equipamentos de informática", organization_name: "Universidade Estadual de Londrina", state: "PR", municipality: "Londrina", modality_code: 6, estimated_value_cents: 7800000, published_at: date(-2), proposal_deadline: date(4), updated_at: date(0), source_url: "https://pncp.gov.br/" },
  { id: "demo-servidores", source: "demo", source_id: "demo-servidores", object: "Fornecimento de servidores, storage e licenças para modernização do datacenter", organization_name: "Secretaria de Administração", state: "SC", municipality: "Florianópolis", modality_code: 6, estimated_value_cents: 46500000, published_at: date(-3), proposal_deadline: date(14), updated_at: date(0), source_url: "https://pncp.gov.br/" },
  { id: "demo-impressoras", source: "demo", source_id: "demo-impressoras", object: "Locação de impressoras multifuncionais com gerenciamento e suprimentos", organization_name: "Instituto Federal do Paraná", state: "PR", municipality: "Cascavel", modality_code: 6, estimated_value_cents: 12600000, published_at: date(-1), proposal_deadline: date(2), updated_at: date(0), source_url: "https://pncp.gov.br/" },
];

export class DemoLicitaLensClient implements LicitaLensAPI {
  private profile?: Profile;
  private analyses = 7;
  private deals: Deal[] = [];
  private followups: Record<string, DealFollowUp[]> = {};
  private notificationPrefs: NotificationPreferencesPayload = {
    push: true,
    email: true,
    whatsapp: false,
    deadline_reminder: true,
  };

	async getConfig(): Promise<PublicConfig> { return { deployment_mode: "demo", public_signup: true, billing: false, push: false, email: false, ai: false }; }
	async logout() { return undefined; }
	async requestEmailVerification(_email: string) {}
	async verifyEmail(_token: string) {}
	async requestPasswordRecovery(_email: string) {}
	async resetPassword(_token: string, _password: string) {}
	async exportAccount(): Promise<Record<string, unknown>> { return { exported_at: new Date().toISOString(), account: { email: "demo@licitalens.local" }, organizations: [] }; }
	async deleteAccount() {}

  async listOpportunities() { return { data: opportunities, next_cursor: null }; }
  async listProfiles() { return { data: this.profile ? [this.profile] : [], next_cursor: null }; }
  async createProfile(input: ProfileInput) { this.profile = this.makeProfile(input); return this.profile; }
  async updateProfile(profileId: string, input: ProfileInput) { this.profile = this.makeProfile(input, profileId); return this.profile; }
  async getOpportunity(id: string) { const item = opportunities.find((value) => value.id === id); if (!item) throw new Error("Oportunidade não encontrada"); return item; }
  async getUsage(): Promise<Usage> { return { plan: "pro", profiles: { used: this.profile ? 1 : 0, limit: 5 }, ai_analyses: { used: this.analyses, limit: 200 }, daily_alerts: { used: 4, limit: 100 } }; }
  async analyze(opportunityId: string, profileId: string): Promise<Analysis> {
    const item = await this.getOpportunity(opportunityId); this.analyses++;
    const semantic = item.id === "demo-impressoras" ? 0.42 : 0.89;
    return { matched: true, match: { profile_id: profileId, opportunity_id: item.id, score: Math.round((semantic * .55 + .86 * .2 + .8 * .15 + .64 * .1) * 100), breakdown: { semantic, recency: .86, financial: .8, competition: .64 }, evidence: ["O objeto contém produtos e serviços do seu perfil", "A região está dentro da área atendida", "O valor está na faixa comercial informada"], explanation: "A oportunidade combina com seu portfólio de tecnologia, está na região atendida e apresenta valor compatível com o perfil configurado.", prompt_version: "demo-v1" } };
  }
  async listOrganizations() { return { data: [{ id: "00000000-0000-0000-0000-000000000001", name: "LicitaLens Demo", status: "active" }] }; }
  async bootstrapOrganization(name: string) { return { id: "00000000-0000-0000-0000-000000000001", name, status: "active" }; }
  async billingHistory() { return { data: [{ id: "demo-1", organization_id: "00000000-0000-0000-0000-000000000001", plan: "pro", status: "active", stripe_event_id: "demo_evt", recorded_at: new Date().toISOString() }] }; }
  async createCheckout(plan: "essential" | "pro") { return { url: `https://checkout.stripe.com/demo/${plan}` }; }
  async createPortal() { return { url: "https://billing.stripe.com/demo/portal" }; }
  async signup(input: { email: string; password: string; full_name: string; organization_name: string; plan: "essential" | "pro"; legal_accepted: true; terms_version: string; privacy_version: string }): Promise<SessionPayload> {
    return {
      access_token: "demo-token",
      account: { id: "demo-account", email: input.email, full_name: input.full_name },
      organization: { id: "00000000-0000-0000-0000-000000000001", name: input.organization_name, status: "active" },
      subscription: { plan: input.plan, status: "trialing" },
    };
  }
  async login(input: { email: string; password: string }): Promise<SessionPayload> {
    return this.signup({ ...input, full_name: "Demo", organization_name: "LicitaLens Demo", plan: "pro", legal_accepted: true, terms_version: "demo", privacy_version: "demo" });
  }
  async getMe() {
    return {
      organization: { id: "00000000-0000-0000-0000-000000000001", name: "LicitaLens Demo", status: "active" },
      subscription: { plan: "pro", status: "trialing", active: true },
    };
  }
  async listDeals() { return { data: this.deals }; }
  async createDeal(input: Partial<Deal> & { title: string; stage?: DealStage }) {
    if (input.opportunity_id) {
      const existing = this.deals.find((deal) => deal.opportunity_id === input.opportunity_id);
      if (existing) return existing;
    }
    const deal: Deal = {
      id: `demo-deal-${this.deals.length + 1}`,
      organization_id: "00000000-0000-0000-0000-000000000001",
      opportunity_id: input.opportunity_id,
      title: input.title,
      buyer_name: input.buyer_name ?? "",
      stage: input.stage ?? "prospecting",
      estimated_value_cents: input.estimated_value_cents ?? 0,
    };
    this.deals = [deal, ...this.deals];
    return deal;
  }
  async updateDeal(dealId: string, input: Partial<Deal> & { stage?: DealStage }) {
    const index = this.deals.findIndex((item) => item.id === dealId);
    if (index < 0) throw new Error("Deal não encontrado");
    const current = this.deals[index]!;
    const updated: Deal = { ...current, ...input, stage: input.stage ?? current.stage };
    this.deals[index] = updated;
    return updated;
  }
  async listDealFollowUps(dealId: string) {
    return { data: this.followups[dealId] ?? [] };
  }
  async addDealFollowUp(dealId: string, note: string) {
    const entry = { id: `demo-fu-${Date.now()}`, deal_id: dealId, note, created_at: new Date().toISOString() };
    this.followups[dealId] = [entry, ...(this.followups[dealId] ?? [])];
    const index = this.deals.findIndex((item) => item.id === dealId);
    if (index >= 0) {
      const current = this.deals[index]!;
      this.deals[index] = { ...current, last_follow_up_note: note };
    }
    return { id: entry.id };
  }
  async getNotificationPreferences() {
    return this.notificationPrefs;
  }
  async updateNotificationPreferences(prefs: NotificationPreferencesPayload) {
    this.notificationPrefs = prefs;
    return prefs;
  }
  async registerPushToken(token: string) {
    return { token };
  }
  async removePushToken(_token: string) {}
  private makeProfile(input: ProfileInput, id = "demo-profile"): Profile { return { ...input, id, organization_id: "00000000-0000-0000-0000-000000000001", created_at: new Date().toISOString() }; }
}
