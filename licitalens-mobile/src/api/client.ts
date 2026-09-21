import type { components } from "./schema";
import type { Deal, DealFollowUp, DealStage, NotificationPreferencesPayload, SessionPayload, SignupPayload } from "./types";
export { userFacingError } from "./errors";
import { messageFromApiBody } from "./errors";
export type { Deal, DealFollowUp, DealStage, NotificationPreferencesPayload, SessionPayload, SignupPayload } from "./types";

export type Opportunity = components["schemas"]["Opportunity"];
export type Profile = components["schemas"]["Profile"];
export type ProfileInput = components["schemas"]["ProfileInput"];
export type Analysis = components["schemas"]["Analysis"];

export type Organization = {
  id: string;
  name: string;
  status: string;
  created_at?: string;
};

export type SubscriptionHistoryEntry = {
  id: string;
  organization_id: string;
  plan: string;
  status: string;
  stripe_event_id?: string;
  recorded_at: string;
};

type UsageItem = { used: number; limit: number };

export type Usage = {
  plan: "essential" | "pro";
  profiles: UsageItem;
  ai_analyses: UsageItem;
  daily_alerts: UsageItem;
};

export type PublicConfig = {
  deployment_mode: "demo" | "self_hosted" | "cloud";
  public_signup: boolean;
  billing: boolean;
  push: boolean;
  email: boolean;
  ai: boolean;
};

type ClientOptions = {
  baseUrl: string;
  organizationId: string;
  accessToken?: string;
  subjectId?: string;
};

export interface LicitaLensAPI {
	getConfig(): Promise<PublicConfig>;
  listOpportunities(profileId?: string): Promise<{ data: Opportunity[]; next_cursor?: string | null }>;
  listProfiles(): Promise<{ data: Profile[]; next_cursor?: string | null }>;
  createProfile(profile: ProfileInput): Promise<Profile>;
  updateProfile(profileId: string, profile: ProfileInput): Promise<Profile>;
  getOpportunity(opportunityId: string): Promise<Opportunity>;
  getUsage(): Promise<Usage>;
  analyze(opportunityId: string, profileId: string): Promise<Analysis>;
  listOrganizations(): Promise<{ data: Organization[] }>;
  bootstrapOrganization(name: string): Promise<Organization>;
  billingHistory(): Promise<{ data: SubscriptionHistoryEntry[] }>;
  createCheckout(plan: "essential" | "pro"): Promise<{ url: string }>;
  createPortal(): Promise<{ url: string }>;
  signup(input: { email: string; password: string; full_name: string; organization_name: string; plan: "essential" | "pro"; legal_accepted: true; terms_version: string; privacy_version: string }): Promise<SignupPayload>;
  login(input: { email: string; password: string }): Promise<SessionPayload>;
	requestEmailVerification(email: string): Promise<void>;
	verifyEmail(token: string): Promise<void>;
	requestPasswordRecovery(email: string): Promise<void>;
	resetPassword(token: string, password: string): Promise<void>;
	exportAccount(): Promise<Record<string, unknown>>;
	deleteAccount(): Promise<void>;
	getMe(): Promise<{ organization: Organization; subscription: { plan: string; status: string; active: boolean } }>;
	logout(): Promise<void>;
  listDeals(): Promise<{ data: Deal[] }>;
  createDeal(input: Partial<Deal> & { title: string; stage?: DealStage }): Promise<Deal>;
  updateDeal(dealId: string, input: Partial<Deal> & { stage?: DealStage }): Promise<Deal>;
  listDealFollowUps(dealId: string): Promise<{ data: DealFollowUp[] }>;
  addDealFollowUp(dealId: string, note: string): Promise<{ id: string }>;
  getNotificationPreferences(): Promise<NotificationPreferencesPayload>;
  updateNotificationPreferences(prefs: NotificationPreferencesPayload): Promise<NotificationPreferencesPayload>;
  registerPushToken(token: string): Promise<{ token: string }>;
  removePushToken(token: string): Promise<void>;
}

export class LicitaLensClient implements LicitaLensAPI {
  constructor(private readonly options: ClientOptions) {}

	getConfig(): Promise<PublicConfig> {
    return this.request("/v1/config", { skipOrganization: true });
	}

	logout(): Promise<void> {
		return this.request("/v1/auth/logout", { method: "POST", skipOrganization: true });
	}

  listOpportunities(profileId?: string): Promise<{ data: Opportunity[]; next_cursor?: string | null }> {
    const query = profileId ? `?profile_id=${encodeURIComponent(profileId)}` : "";
    return this.request(`/v1/opportunities${query}`);
  }

  listProfiles(): Promise<{ data: Profile[]; next_cursor?: string | null }> {
    return this.request("/v1/profiles");
  }

  createProfile(profile: ProfileInput): Promise<Profile> {
    return this.request("/v1/profiles", { method: "POST", body: JSON.stringify(profile) });
  }

  updateProfile(profileId: string, profile: ProfileInput): Promise<Profile> {
    return this.request(`/v1/profiles/${encodeURIComponent(profileId)}`, {
      method: "PUT",
      body: JSON.stringify(profile),
    });
  }

  getOpportunity(opportunityId: string): Promise<Opportunity> {
    return this.request(`/v1/opportunities/${encodeURIComponent(opportunityId)}`);
  }

  getUsage(): Promise<Usage> {
    return this.request("/v1/usage");
  }

  analyze(opportunityId: string, profileId: string): Promise<Analysis> {
    return this.request(`/v1/opportunities/${encodeURIComponent(opportunityId)}/analysis`, {
      method: "POST",
      body: JSON.stringify({ profile_id: profileId }),
    });
  }

  listOrganizations(): Promise<{ data: Organization[] }> {
    return this.request("/v1/account/organizations", { skipOrganization: true });
  }

  bootstrapOrganization(name: string): Promise<Organization> {
    return this.request("/v1/account/bootstrap", {
      method: "POST",
      skipOrganization: true,
      body: JSON.stringify({ organization_name: name }),
    });
  }

  billingHistory(): Promise<{ data: SubscriptionHistoryEntry[] }> {
    return this.request("/v1/billing/history");
  }

  createCheckout(plan: "essential" | "pro"): Promise<{ url: string }> {
    return this.request("/v1/billing/checkout", { method: "POST", body: JSON.stringify({ plan }) });
  }

  createPortal(): Promise<{ url: string }> {
    return this.request("/v1/billing/portal", { method: "POST", body: JSON.stringify({}) });
  }

  signup(input: { email: string; password: string; full_name: string; organization_name: string; plan: "essential" | "pro"; legal_accepted: true; terms_version: string; privacy_version: string }): Promise<SignupPayload> {
    return this.request("/v1/auth/signup", { method: "POST", skipOrganization: true, body: JSON.stringify(input) });
  }

  login(input: { email: string; password: string }): Promise<SessionPayload> {
    return this.request("/v1/auth/login", { method: "POST", skipOrganization: true, body: JSON.stringify(input) });
  }

	requestEmailVerification(email: string): Promise<void> {
		return this.request("/v1/auth/email-verification", { method: "POST", skipOrganization: true, body: JSON.stringify({ email }) });
	}

	verifyEmail(token: string): Promise<void> {
		return this.request("/v1/auth/verify-email", { method: "POST", skipOrganization: true, body: JSON.stringify({ token }) });
	}

	requestPasswordRecovery(email: string): Promise<void> {
		return this.request("/v1/auth/password-recovery", { method: "POST", skipOrganization: true, body: JSON.stringify({ email }) });
	}

	resetPassword(token: string, password: string): Promise<void> {
		return this.request("/v1/auth/password-reset", { method: "POST", skipOrganization: true, body: JSON.stringify({ token, password }) });
	}

	exportAccount(): Promise<Record<string, unknown>> {
		return this.request("/v1/account/export", { skipOrganization: true });
	}

	deleteAccount(): Promise<void> {
		return this.request("/v1/account", { method: "DELETE", skipOrganization: true, body: JSON.stringify({ confirmation: "DELETE" }) });
	}

  getMe(): Promise<{ organization: Organization; subscription: { plan: string; status: string; active: boolean } }> {
    return this.request("/v1/me", { skipOrganization: true });
  }

  listDeals(): Promise<{ data: Deal[] }> {
    return this.request("/v1/deals");
  }

  createDeal(input: Partial<Deal> & { title: string; stage?: DealStage }): Promise<Deal> {
    return this.request("/v1/deals", { method: "POST", body: JSON.stringify(input) });
  }

  updateDeal(dealId: string, input: Partial<Deal> & { stage?: DealStage }): Promise<Deal> {
    return this.request(`/v1/deals/${encodeURIComponent(dealId)}`, { method: "PATCH", body: JSON.stringify(input) });
  }

  listDealFollowUps(dealId: string): Promise<{ data: DealFollowUp[] }> {
    return this.request(`/v1/deals/${encodeURIComponent(dealId)}/followups`);
  }

  addDealFollowUp(dealId: string, note: string): Promise<{ id: string }> {
    return this.request(`/v1/deals/${encodeURIComponent(dealId)}/followups`, { method: "POST", body: JSON.stringify({ note }) });
  }

  getNotificationPreferences(): Promise<NotificationPreferencesPayload> {
    return this.request("/v1/account/notification-preferences");
  }

  updateNotificationPreferences(prefs: NotificationPreferencesPayload): Promise<NotificationPreferencesPayload> {
    return this.request("/v1/account/notification-preferences", { method: "PATCH", body: JSON.stringify(prefs) });
  }

  registerPushToken(token: string): Promise<{ token: string }> {
    return this.request("/v1/account/push-token", { method: "PUT", body: JSON.stringify({ token }) });
  }

  removePushToken(token: string): Promise<void> {
    return this.request("/v1/account/push-token", { method: "DELETE", body: JSON.stringify({ token }) });
  }

  private async request<T>(path: string, init: RequestInit & { skipOrganization?: boolean } = {}): Promise<T> {
	const headers: Record<string, string> = {
      Accept: "application/json",
      "Content-Type": "application/json",
      ...(this.options.accessToken ? { Authorization: `Bearer ${this.options.accessToken}` } : {}),
      ...((init.headers as Record<string, string>) ?? {}),
	};
		if (!this.options.accessToken && typeof document !== "undefined" && ["POST", "PUT", "PATCH", "DELETE"].includes(init.method ?? "GET")) {
			const csrf = document.cookie.split("; ").find((item) => item.startsWith("licitalens_csrf="))?.split("=")[1];
			if (csrf) headers["X-CSRF-Token"] = decodeURIComponent(csrf);
		}
    if (!init.skipOrganization) {
      headers["X-Organization-ID"] = this.options.organizationId;
    }
    if (this.options.subjectId) {
      headers["X-User-ID"] = this.options.subjectId;
    }
    let response: Response;
    try {
		response = await fetch(`${this.options.baseUrl}${path}`, {
			...init,
			headers,
			credentials: "include",
      });
    } catch {
      throw new Error("Sem conexão com a API. Confira Wi‑Fi e se o backend está no ar (make commercial-up).");
    }
    let body: unknown;
    const raw = await response.text();
    if (raw) {
      try {
        body = JSON.parse(raw);
      } catch {
        body = raw;
      }
    }
    if (!response.ok) {
      throw new Error(messageFromApiBody(body, response.status));
    }
    if (response.status === 204 || !raw) {
      return undefined as T;
    }
    return JSON.parse(raw) as T;
  }
}
