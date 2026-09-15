import { ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ActivityIndicator,
  Animated,
  FlatList,
  Linking,
  Modal,
  PanResponder,
  Pressable,
  RefreshControl,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Switch,
  Text,
  TextInput,
  useWindowDimensions,
  View,
} from "react-native";
import { StatusBar as ExpoStatusBar } from "expo-status-bar";
import { Analysis, LicitaLensClient, NotificationPreferencesPayload, Opportunity, Profile, ProfileInput, SessionPayload, SubscriptionHistoryEntry, Usage, userFacingError } from "../api/client";
import { Preferences, clearSession, defaultPreferences, loadLocalState, saveOnboarded, savePreferences, saveSession } from "../storage";
import { DEAL_STAGES, Deal, DealFollowUp, DealStage } from "../api/types";
import { colors } from "../theme";
import { resolveExpoPushToken } from "../notifications/push";
import { GuideProgress, loadGuideProgress, resetGuideProgress, saveGuideProgress } from "../guideProgress";
import { InAppGuide, APP_PLAYBOOK, AUTH_PLAYBOOK, PlaybookBar, advanceOnStepId, stepForIndex } from "./InAppGuide";
import {
  SpotlightRing,
  spotlightAddPipeline,
  spotlightAnalyze,
  spotlightAuthCreateAccount,
  spotlightAuthOnboard,
  spotlightAuthTrial,
  spotlightEmailSwitch,
  spotlightFollowUp,
  spotlightNavTab,
  spotlightRadarCard,
  spotlightSaveProfile,
} from "./PlaybookSpotlight";
import { LegalModal } from "./LegalModal";
import { LegalDoc, LEGAL_VERSION } from "../legal/content";

const apiUrl = process.env.EXPO_PUBLIC_API_URL ?? "http://localhost:8080";

function prefsFromApi(payload: NotificationPreferencesPayload): Preferences {
  return {
    push: payload.push,
    email: payload.email,
    whatsapp: payload.whatsapp,
    deadlineReminder: payload.deadline_reminder,
  };
}

function prefsToApi(preferences: Preferences): NotificationPreferencesPayload {
  return {
    push: preferences.push,
    email: preferences.email,
    whatsapp: preferences.whatsapp,
    deadline_reminder: preferences.deadlineReminder,
  };
}

type Phase = "welcome" | "login" | "signup" | "activate" | "onboarding" | "app";
type Tab = "overview" | "dashboard" | "radar" | "alerts" | "activity" | "profile" | "plan" | "settings";

const NAV_ITEMS: Array<{ key: Tab; icon: string; label: string; description: string }> = [
  { key: "overview", icon: "◫", label: "Visão", description: "Indicadores e prioridades" },
  { key: "dashboard", icon: "▦", label: "Pipeline", description: "Gestão das oportunidades" },
  { key: "radar", icon: "⌕", label: "Radar", description: "Licitações encontradas" },
  { key: "alerts", icon: "✦", label: "Alertas", description: "Critérios e canais" },
  { key: "activity", icon: "↗", label: "Atividades", description: "Histórico da operação" },
  { key: "profile", icon: "◎", label: "Perfil", description: "Aderência comercial" },
  { key: "settings", icon: "○", label: "Conta", description: "Preferências e acesso" },
];

function tabTitle(tab: Tab) {
  return tab === "overview"
    ? "Visão comercial"
    : tab === "dashboard"
      ? "Pipeline comercial"
      : tab === "radar"
        ? "Radar de licitações"
        : tab === "alerts"
          ? "Central de alertas"
          : tab === "activity"
            ? "Atividades"
            : tab === "profile"
              ? "Perfil comercial"
              : tab === "plan"
                ? "Assinatura"
                : "Conta";
}

function normalizedSubscriptionStatus(subscription: unknown): string {
  if (!subscription || typeof subscription !== "object") return "trialing";
  const raw = subscription as Record<string, unknown>;
  const status = raw.status ?? raw.Status;
  return typeof status === "string" && status.trim() ? status : "trialing";
}

function opportunitySourceLabel(source?: string): string {
  if (source === "pncp") return "PNCP"
  if (source === "compras_gov") return "Compras.gov.br"
  if (source === "demo") return "Demonstração"
  return "Fonte pública"
}

function opportunityFreshness(updatedAt?: string, publishedAt?: string): string {
  const value = updatedAt || publishedAt;
  if (!value) return "Atualização não informada";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Atualização não informada";
  return `Atualizado em ${date.toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" })}`;
}

function ComplianceNotice({ compact = false }: { compact?: boolean }) {
  return (
    <View style={[styles.complianceNotice, compact && styles.complianceNoticeCompact]}>
      <Text style={styles.complianceNoticeTitle}>Apoio à decisão comercial</Text>
      <Text style={styles.complianceNoticeText}>
        O radar organiza dados públicos e o score indica aderência ao seu perfil. Não confirma habilitação, classificação ou chance de vitória. Confira sempre o edital e a fonte oficial.
      </Text>
    </View>
  );
}

function LegalFooter({ onOpen }: { onOpen: (doc: LegalDoc) => void }) {
  return (
    <View style={styles.legalFooter}>
      <Text style={styles.legalFooterMuted}>Documentos legais (v{LEGAL_VERSION})</Text>
      <View style={styles.legalFooterLinks}>
        <Pressable onPress={() => onOpen("terms")}><Text style={styles.legalLink}>Termos de Uso</Text></Pressable>
        <Text style={styles.legalFooterMuted}>·</Text>
        <Pressable onPress={() => onOpen("privacy")}><Text style={styles.legalLink}>Privacidade (LGPD)</Text></Pressable>
      </View>
    </View>
  );
}

export default function SaasFlow() {
  const { width } = useWindowDimensions();
  const desktop = width >= 1024;
  const [phase, setPhase] = useState<Phase>("welcome");
  const [tab, setTab] = useState<Tab>("overview");
  const [accessToken, setAccessToken] = useState<string>();
  const [organizationId, setOrganizationId] = useState("");
  const [organizationName, setOrganizationName] = useState("");
  const [accountName, setAccountName] = useState("");
  const [accountEmail, setAccountEmail] = useState("");
  const [subscriptionStatus, setSubscriptionStatus] = useState("trialing");
  const [profile, setProfile] = useState<Profile>();
  const [opportunities, setOpportunities] = useState<Opportunity[]>([]);
  const [deals, setDeals] = useState<Deal[]>([]);
  const [usage, setUsage] = useState<Usage>();
  const [billingHistory, setBillingHistory] = useState<SubscriptionHistoryEntry[]>([]);
  const [selectedOpportunity, setSelectedOpportunity] = useState<Opportunity>();
  const [analysis, setAnalysis] = useState<Analysis>();
  const [refreshing, setRefreshing] = useState(false);
  const [dashboardMode, setDashboardMode] = useState<"kanban" | "list">("kanban");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [preferences, setPreferences] = useState<Preferences>(defaultPreferences);
  const [guideOpen, setGuideOpen] = useState(false);
  const [legalDoc, setLegalDoc] = useState<LegalDoc | null>(null);
  const openLegal = useCallback((doc: LegalDoc) => setLegalDoc(doc), []);
  const closeLegal = useCallback(() => setLegalDoc(null), []);
  const [guideProgress, setGuideProgress] = useState<GuideProgress>({ active: false, stepIndex: 0, completed: [] });

  const patchGuideProgress = useCallback((next: GuideProgress) => {
    setGuideProgress(next);
    void saveGuideProgress(next);
  }, []);

  const bumpGuideStep = useCallback((stepId: string) => {
    setGuideProgress((prev) => {
      const next = advanceOnStepId(prev, stepId, "app");
      if (
        next.stepIndex === prev.stepIndex &&
        next.active === prev.active &&
        next.completed.length === prev.completed.length
      ) {
        return prev;
      }
      void saveGuideProgress(next);
      return next;
    });
  }, []);

  const reportError = useCallback((cause: unknown, fallback: string) => {
    setError(userFacingError(cause, fallback));
  }, []);

  const runAction = useCallback(
    async (action: () => Promise<void>, fallback: string) => {
      try {
        setError(undefined);
        await action();
      } catch (cause) {
        reportError(cause, fallback);
      }
    },
    [reportError],
  );

  const opportunityById = useMemo(() => {
    const map = new Map<string, Opportunity>();
    for (const item of opportunities) map.set(item.id, item);
    return map;
  }, [opportunities]);

  const client = useMemo(
    () => new LicitaLensClient({ baseUrl: apiUrl, organizationId, accessToken }),
    [accessToken, organizationId],
  );

  const subscriptionOk = (status: string) => status === "trialing" || status === "active";

  const applySession = useCallback(async (session: SessionPayload) => {
    try {
      const sessionSubscriptionStatus = normalizedSubscriptionStatus(session.subscription);
      setAccessToken(session.access_token);
      setOrganizationId(session.organization.id);
      setOrganizationName(session.organization.name);
      setAccountName(session.account.full_name);
      setAccountEmail(session.account.email);
      setSubscriptionStatus(sessionSubscriptionStatus);
      await saveSession(session.access_token, session.organization.id, session.organization.name, session.account.email);
      if (!subscriptionOk(sessionSubscriptionStatus)) {
        setPhase("activate");
        return;
      }
      const authed = new LicitaLensClient({
        baseUrl: apiUrl,
        organizationId: session.organization.id,
        accessToken: session.access_token,
      });
      const profiles = await authed.listProfiles();
      if (profiles.data[0]) {
        setProfile(profiles.data[0]);
        await saveOnboarded(true);
        setPhase("app");
        return;
      }
      setPhase("onboarding");
    } catch (cause) {
      reportError(cause, "Conta criada, mas não foi possível carregar a organização.");
      throw cause;
    }
  }, [reportError]);

  const playbookUiActive = guideProgress.active && !guideOpen;
  const currentAppStep = phase === "app" && playbookUiActive ? stepForIndex("app", guideProgress.stepIndex) : null;
  const currentAuthStep = phase !== "app" && playbookUiActive ? stepForIndex("auth", guideProgress.stepIndex) : null;
  const navSpotlight =
    currentAppStep?.tab && tab !== currentAppStep.tab ? (spotlightNavTab(currentAppStep, tab) ?? null) : null;
  const modalOpen = Boolean(selectedOpportunity);
  const accountDisplayName = accountName || accountEmail.split("@")[0] || "Conta";
  const followUpDealId = deals.find((d) => d.opportunity_id)?.id;

  useEffect(() => {
    void loadGuideProgress().then(setGuideProgress);
  }, []);

  useEffect(() => {
    void loadLocalState().then(async (local) => {
      if (!local.accessToken) {
        setLoading(false);
        return;
      }
      setAccessToken(local.accessToken);
      setOrganizationId(local.organizationId ?? "");
      setOrganizationName(local.organizationName ?? "");
      setPreferences(local.preferences);
      setAccountEmail(local.accountEmail);
      setAccountName(local.accountEmail?.split("@")[0] ?? "");
      try {
        const meClient = new LicitaLensClient({ baseUrl: apiUrl, organizationId: local.organizationId ?? "", accessToken: local.accessToken });
        const me = await meClient.getMe();
        setOrganizationId(me.organization.id);
        setOrganizationName(me.organization.name);
        const currentSubscriptionStatus = normalizedSubscriptionStatus(me.subscription);
        setSubscriptionStatus(currentSubscriptionStatus);
        if (!me.subscription.active && !subscriptionOk(currentSubscriptionStatus)) {
          setPhase("activate");
          return;
        }
        const profiles = await new LicitaLensClient({
          baseUrl: apiUrl,
          organizationId: me.organization.id,
          accessToken: local.accessToken,
        }).listProfiles();
        if (profiles.data[0]) {
          setProfile(profiles.data[0]);
          setPhase("app");
        } else {
          setPhase("onboarding");
        }
      } catch {
        setError("Sessão inválida ou API indisponível. Entre novamente.");
        setPhase("welcome");
      } finally {
        setLoading(false);
      }
    });
  }, []);

  const loadAppData = useCallback(async (refresh = false) => {
    setError(undefined);
    if (refresh) setRefreshing(true);
    try {
      const profiles = await client.listProfiles();
      const profileId = profiles.data[0]?.id;
      const [feed, dealPage, currentUsage, history] = await Promise.all([
        client.listOpportunities(profileId),
        client.listDeals(),
        client.getUsage(),
        client.billingHistory().catch(() => ({ data: [] as SubscriptionHistoryEntry[] })),
      ]);
      setProfile(profiles.data[0]);
      setOpportunities(feed.data);
      setDeals(dealPage.data);
      setUsage(currentUsage);
      setBillingHistory(history.data);
      try {
        const remote = await client.getNotificationPreferences();
        const synced = prefsFromApi(remote);
        setPreferences(synced);
        await savePreferences(synced);
      } catch {
        /* mantém preferências locais */
      }
    } catch (cause) {
      setError(userFacingError(cause, "Falha ao carregar dados."));
    } finally {
      if (refresh) setRefreshing(false);
    }
  }, [client]);

  const addOpportunityToPipeline = useCallback(
    async (item: Opportunity) => {
      try {
        setError(undefined);
        await client.createDeal({
          title: item.object.length > 180 ? `${item.object.slice(0, 177)}…` : item.object,
          opportunity_id: item.id,
          buyer_name: item.organization_name,
          estimated_value_cents: item.estimated_value_cents ?? 0,
          stage: "analysis",
        });
        bumpGuideStep("app-4");
        await loadAppData();
        setTab("dashboard");
      } catch (cause) {
        reportError(cause, "Não foi possível adicionar ao pipeline.");
      }
    },
    [bumpGuideStep, client, loadAppData, reportError],
  );

  const openLinkedOpportunity = useCallback(
    async (deal: Deal) => {
      const id = deal.opportunity_id?.trim();
      if (!id) return;
      setAnalysis(undefined);
      const cached = opportunityById.get(id);
      if (cached) {
        setSelectedOpportunity(cached);
        return;
      }
      try {
        setSelectedOpportunity(await client.getOpportunity(id));
      } catch {
        setError("Não foi possível carregar a licitação vinculada a este card.");
      }
    },
    [client, opportunityById],
  );

  useEffect(() => {
    if (phase !== "app") return;
    void loadLocalState().then((local) => setPreferences(local.preferences));
  }, [phase]);

  useEffect(() => {
    if (phase === "app") void loadAppData();
  }, [phase, loadAppData]);

  async function finishOnboarding(input: ProfileInput) {
    try {
      const created = await client.createProfile(input);
      setProfile(created);
      await saveOnboarded(true);
      setPhase("app");
      const start: GuideProgress = { active: true, stepIndex: 0, completed: [] };
      patchGuideProgress(start);
      setGuideOpen(true);
    } catch (cause) {
      reportError(cause, "Não foi possível criar o perfil.");
      throw cause;
    }
  }

  const legalModal = <LegalModal doc={legalDoc} visible={legalDoc !== null} onClose={closeLegal} />;

  if (loading) {
    return (
      <SafeAreaView style={styles.center}>
        <ActivityIndicator color={colors.blue} />
        {legalModal}
      </SafeAreaView>
    );
  }

  if (phase === "welcome") {
    return (
      <View style={styles.authFlow}>
        <AuthShell title="LicitaLens SaaS" copy="Descubra licitações, faça follow-up comercial e feche contratos públicos em um só lugar.">
          <SpotlightRing active={spotlightAuthCreateAccount(currentAuthStep, phase)}>
            <PrimaryButton label="Criar conta" onPress={() => setPhase("signup")} />
          </SpotlightRing>
          <SecondaryButton label="Já tenho conta" onPress={() => setPhase("login")} />
          <SecondaryButton
            label="Roteiro passo a passo"
            onPress={() => {
              patchGuideProgress({ active: true, stepIndex: 0, completed: guideProgress.completed });
              setGuideOpen(true);
            }}
          />
          <LegalFooter onOpen={openLegal} />
        </AuthShell>
        {legalModal}
        <InAppGuide
          visible={guideOpen}
          mode="auth"
          progress={guideProgress}
          onProgressChange={patchGuideProgress}
          onClose={() => setGuideOpen(false)}
          onGoToTab={(tab) => setTab(tab)}
          onGoToPhase={(next) => setPhase(next)}
        />
        {currentAuthStep && (
          <PlaybookBar
            visible={playbookUiActive}
            step={currentAuthStep}
            index={guideProgress.stepIndex}
            total={AUTH_PLAYBOOK.length}
            onOpen={() => setGuideOpen(true)}
            onDismiss={() => patchGuideProgress({ ...guideProgress, active: false })}
          />
        )}
      </View>
    );
  }

  if (phase === "signup") {
    return (
      <>
        <SignUpScreen onBack={() => setPhase("welcome")} onComplete={applySession} onOpenLegal={openLegal} />
        {legalModal}
      </>
    );
  }

  if (phase === "login") {
    return <LoginScreen onBack={() => setPhase("welcome")} onComplete={applySession} />;
  }

  if (phase === "activate") {
    return (
      <View style={styles.authFlow}>
        <AuthShell title="Ative sua assinatura" copy={`Plano em status ${subscriptionStatus}. Continue com o trial ou escolha um plano pago.`}>
          {error && <Text style={styles.errorInline}>{error}</Text>}
          <SpotlightRing active={spotlightAuthTrial(currentAuthStep, phase)}>
            <PrimaryButton label="Continuar com trial" onPress={() => { setError(undefined); setPhase("onboarding"); }} />
          </SpotlightRing>
          <PrimaryButton
            label="Assinar Pro (Stripe)"
            onPress={() =>
              void runAction(async () => {
                const { url } = await client.createCheckout("pro");
                await Linking.openURL(url);
              }, "Checkout indisponível. Configure Stripe no backend ou use o trial.")
            }
          />
          <SecondaryButton label="Voltar ao login" onPress={async () => { await clearSession(); setPhase("welcome"); }} />
        </AuthShell>
        {currentAuthStep && (
          <PlaybookBar
            visible={playbookUiActive}
            step={currentAuthStep}
            index={guideProgress.stepIndex}
            total={AUTH_PLAYBOOK.length}
            onOpen={() => setGuideOpen(true)}
            onDismiss={() => patchGuideProgress({ ...guideProgress, active: false })}
          />
        )}
      </View>
    );
  }

  if (phase === "onboarding") {
    return (
      <View style={styles.authFlow}>
        <ProfileOnboarding
          organizationName={organizationName}
          highlightStart={spotlightAuthOnboard(currentAuthStep, phase)}
          onComplete={finishOnboarding}
        />
        {currentAuthStep && (
          <PlaybookBar
            visible={playbookUiActive}
            step={currentAuthStep}
            index={guideProgress.stepIndex}
            total={AUTH_PLAYBOOK.length}
            onOpen={() => setGuideOpen(true)}
            onDismiss={() => patchGuideProgress({ ...guideProgress, active: false })}
          />
        )}
      </View>
    );
  }

  return (
    <SafeAreaView style={styles.app}>
      <ExpoStatusBar style={desktop ? "dark" : "light"} />
      <View style={styles.appShell}>
        {desktop && (
          <DesktopSidebar
            active={tab}
            accountName={accountDisplayName}
            organizationName={organizationName}
            subscriptionStatus={subscriptionStatus}
            onChange={setTab}
          />
        )}
        <View style={styles.workspace}>
          <View style={[styles.header, desktop && styles.headerDesktop]}>
            <View>
              <Text style={[styles.headerEyebrow, desktop && styles.headerEyebrowDesktop]}>{organizationName}</Text>
              <Text style={[styles.headerTitle, desktop && styles.headerTitleDesktop]}>{tabTitle(tab)}</Text>
            </View>
            <View style={styles.headerActions}>
              {desktop && (
                <View style={styles.livePill}>
                  <View style={styles.liveDot} />
                  <Text style={styles.livePillText}>Operação online</Text>
                </View>
              )}
              <Text style={[styles.headerUser, desktop && styles.headerUserDesktop]}>{desktop ? accountDisplayName.split(" ")[0] : accountDisplayName.slice(0, 1).toUpperCase()}</Text>
            </View>
          </View>
          <View style={styles.workspaceContent}>
      {error && <Text style={styles.error}>{error}</Text>}
      <PlaybookBar
        visible={guideProgress.active && !guideOpen}
        step={stepForIndex("app", guideProgress.stepIndex)}
        index={guideProgress.stepIndex}
        total={APP_PLAYBOOK.length}
        onOpen={() => setGuideOpen(true)}
        onDismiss={() => patchGuideProgress({ ...guideProgress, active: false })}
      />
      {tab === "overview" && (
        <InsightsDashboard
          desktop={desktop}
          deals={deals}
          opportunities={opportunities}
          usage={usage}
          onOpenPipeline={() => setTab("dashboard")}
          onOpenRadar={() => setTab("radar")}
          onOpenAlerts={() => setTab("alerts")}
          onOpenActivity={() => setTab("activity")}
        />
      )}
      {tab === "dashboard" && (
        <DashboardScreen
          desktop={desktop}
          deals={deals}
          mode={dashboardMode}
          onModeChange={setDashboardMode}
          highlightFollowUpDealId={spotlightFollowUp(currentAppStep, tab) ? followUpDealId : undefined}
          loadFollowUps={(dealId) => client.listDealFollowUps(dealId).then((page) => page.data)}
          onOpenOpportunity={(deal) => void openLinkedOpportunity(deal)}
          onMove={(deal, stage) =>
            runAction(async () => {
              const snapshot = deals;
              setDeals((current) => current.map((item) => item.id === deal.id ? { ...item, stage } : item));
              try {
                await client.updateDeal(deal.id, { stage });
                await loadAppData();
              } catch (cause) {
                setDeals(snapshot);
                throw cause;
              }
            }, "Não foi possível atualizar o card.")
          }
          onMarkLost={(deal) =>
            runAction(async () => {
              await client.updateDeal(deal.id, { stage: "lost" });
              await loadAppData();
            }, "Não foi possível marcar como perdido.")
          }
          onFollowUp={(deal, note) =>
            runAction(async () => {
              await client.addDealFollowUp(deal.id, note);
              bumpGuideStep("app-5");
              await loadAppData();
            }, "Não foi possível salvar o follow-up.")
          }
          onCreate={(title) =>
            runAction(async () => {
              await client.createDeal({ title, stage: "prospecting", buyer_name: "" });
              await loadAppData();
            }, "Não foi possível criar o card.")
          }
        />
      )}
      {tab === "radar" && (
        <RadarScreen
          desktop={desktop}
          opportunities={opportunities}
          deals={deals}
          profile={profile}
          preferences={preferences}
          refreshing={refreshing}
          onRefresh={() => void loadAppData(true)}
          highlightAlert={currentAppStep?.id === "app-alert" && tab === "radar"}
          highlightFirstCard={spotlightRadarCard(currentAppStep, tab, modalOpen)}
          onSelect={(item) => {
            setSelectedOpportunity(item);
            bumpGuideStep("app-2");
          }}
          onAddToPipeline={addOpportunityToPipeline}
          onSaveAlert={async ({ keywords, states, email }) => {
            if (!profile) return;
            const saved = await client.updateProfile(profile.id, {
              name: profile.name,
              description: profile.description,
              keywords,
              categories: profile.categories,
              states,
              municipalities: profile.municipalities,
              modalities: profile.modalities,
              required_terms: profile.required_terms,
              minimum_value_cents: profile.minimum_value_cents,
              maximum_value_cents: profile.maximum_value_cents,
              excluded_terms: profile.excluded_terms,
            });
            setProfile(saved);
            const nextPreferences = { ...preferences, email };
            setPreferences(nextPreferences);
            await savePreferences(nextPreferences);
            await client.updateNotificationPreferences(prefsToApi(nextPreferences));
            bumpGuideStep("app-alert");
            await loadAppData();
          }}
        />
      )}
      {tab === "alerts" && (
        <AlertsScreen
          desktop={desktop}
          profile={profile}
          preferences={preferences}
          opportunities={opportunities}
          onOpenRadar={() => setTab("radar")}
          onOpenSettings={() => setTab("settings")}
        />
      )}
      {tab === "activity" && (
        <ActivityScreen desktop={desktop} deals={deals} opportunities={opportunities} />
      )}
      {tab === "profile" && !profile && (
        <View style={{ padding: 16 }}>
          <Text style={styles.muted}>Nenhum perfil comercial. Conclua o onboarding ou recarregue o app.</Text>
        </View>
      )}
      {tab === "profile" && profile && (
        <ProfileEditorScreen
          profile={profile}
          highlightSave={spotlightSaveProfile(currentAppStep, tab)}
          onSave={async (input) => {
            const saved = await client.updateProfile(profile.id, input);
            setProfile(saved);
            bumpGuideStep("app-1");
            await loadAppData();
            return saved;
          }}
        />
      )}
      {tab === "plan" && usage && (
        <PlanScreen
          usage={usage}
          organizationName={organizationName}
          history={billingHistory}
          onCheckout={(plan) =>
            runAction(async () => {
              const { url } = await client.createCheckout(plan);
              await Linking.openURL(url);
            }, "Checkout indisponível.")
          }
          onPortal={() =>
            runAction(async () => {
              const { url } = await client.createPortal();
              await Linking.openURL(url);
            }, "Portal de cobrança indisponível.")
          }
        />
      )}
      {tab === "settings" && (
        <SettingsScreen
          accountName={accountName}
          emailHint={accountEmail || organizationName}
          preferences={preferences}
          highlightEmailSwitch={spotlightEmailSwitch(currentAppStep, tab)}
          onChange={async (next) => {
            setPreferences(next);
            await savePreferences(next);
            try {
              await client.updateNotificationPreferences(prefsToApi(next));
              bumpGuideStep("app-6");
              if (next.push) {
                const token = await resolveExpoPushToken();
                if (token) {
                  await client.registerPushToken(token);
                }
              }
            } catch {
              setError("Preferências salvas no dispositivo, mas não sincronizaram com o servidor.");
            }
          }}
          onSignOut={async () => {
            await clearSession();
            await resetGuideProgress();
            setGuideProgress({ active: false, stepIndex: 0, completed: [] });
            setPhase("welcome");
            setAccessToken(undefined);
          }}
          onOpenGuide={() => {
            patchGuideProgress({ ...guideProgress, active: true });
            setGuideOpen(true);
          }}
          onOpenPlan={() => setTab("plan")}
          onOpenLegal={openLegal}
        />
      )}
      {legalModal}
      <InAppGuide
        visible={guideOpen}
        mode="app"
        progress={guideProgress}
        onProgressChange={patchGuideProgress}
        onClose={() => setGuideOpen(false)}
        onGoToTab={(tab) => setTab(tab)}
        onGoToPhase={(next) => setPhase(next)}
      />
          </View>
          {!desktop && <BottomNav active={tab} onChange={setTab} spotlightTab={navSpotlight} />}
        </View>
      </View>
      <OpportunityDetailModal
        item={selectedOpportunity}
        analysis={analysis}
        inPipeline={selectedOpportunity ? deals.some((deal) => deal.opportunity_id === selectedOpportunity.id) : false}
        highlightAnalyze={spotlightAnalyze(currentAppStep, modalOpen)}
        highlightPipeline={spotlightAddPipeline(currentAppStep, modalOpen, selectedOpportunity ? deals.some((deal) => deal.opportunity_id === selectedOpportunity.id) : false)}
        onClose={() => {
          setSelectedOpportunity(undefined);
          setAnalysis(undefined);
        }}
        onAnalyze={async () => {
          if (!selectedOpportunity || !profile) return;
          try {
            const result = await client.analyze(selectedOpportunity.id, profile.id);
            setAnalysis(result);
            bumpGuideStep("app-3");
            await loadAppData();
          } catch (cause) {
            reportError(cause, "Análise indisponível.");
          }
        }}
        onAddToPipeline={async () => {
          if (!selectedOpportunity) return;
          await addOpportunityToPipeline(selectedOpportunity);
          setSelectedOpportunity(undefined);
          setAnalysis(undefined);
        }}
        onOpenSource={(url) => void Linking.openURL(url)}
      />
    </SafeAreaView>
  );
}

function InsightsDashboard({
  desktop,
  deals,
  opportunities,
  usage,
  onOpenPipeline,
  onOpenRadar,
  onOpenAlerts,
  onOpenActivity,
}: {
  desktop: boolean;
  deals: Deal[];
  opportunities: Opportunity[];
  usage?: Usage;
  onOpenPipeline: () => void;
  onOpenRadar: () => void;
  onOpenAlerts: () => void;
  onOpenActivity: () => void;
}) {
  const activeDeals = deals.filter((deal) => deal.stage !== "won" && deal.stage !== "lost");
  const wonDeals = deals.filter((deal) => deal.stage === "won");
  const closedDeals = deals.filter((deal) => deal.stage === "won" || deal.stage === "lost");
  const pipelineValue = activeDeals.reduce((total, deal) => total + (deal.estimated_value_cents ?? 0), 0);
  const conversion = closedDeals.length ? Math.round((wonDeals.length / closedDeals.length) * 100) : 0;
  const urgent = opportunities
    .filter((item) => {
      const days = daysUntil(item.proposal_deadline);
      return days >= 0 && days <= 7;
    })
    .sort((a, b) => daysUntil(a.proposal_deadline) - daysUntil(b.proposal_deadline));
  const stages: DealStage[] = ["prospecting", "analysis", "proposal", "negotiation"];
  const stageCounts = stages.map((stage) => ({
    stage,
    label: DEAL_STAGES.find((item) => item.key === stage)?.label ?? stage,
    count: deals.filter((deal) => deal.stage === stage).length,
  }));
  const maxStageCount = Math.max(1, ...stageCounts.map((item) => item.count));
  const insight = urgent.length
    ? `${urgent.length} ${urgent.length === 1 ? "licitação encerra" : "licitações encerram"} nos próximos 7 dias.`
    : activeDeals.length
      ? `${activeDeals.length} ${activeDeals.length === 1 ? "negociação ativa" : "negociações ativas"} aguardam avanço.`
      : "Seu pipeline está pronto para receber novas oportunidades.";

  return (
    <ScrollView contentContainerStyle={[styles.dashboardScroll, desktop && styles.dashboardScrollDesktop]} showsVerticalScrollIndicator={false}>
      <View style={[styles.dashboardHero, desktop && styles.dashboardHeroDesktop]}>
        <Text style={styles.dashboardEyebrow}>PANORAMA DE HOJE</Text>
        <Text style={styles.dashboardTitle}>Decida o próximo movimento.</Text>
        <Text style={styles.dashboardCopy}>{insight}</Text>
        <View style={styles.dashboardHeroActions}>
          <Pressable onPress={onOpenPipeline} style={styles.dashboardPrimaryAction}>
            <Text style={styles.dashboardPrimaryActionText}>Abrir pipeline</Text>
          </Pressable>
          <Pressable onPress={onOpenRadar} style={styles.dashboardSecondaryAction}>
            <Text style={styles.dashboardSecondaryActionText}>Ver radar</Text>
          </Pressable>
        </View>
      </View>

      <View style={[styles.quickActionsGrid, desktop && styles.quickActionsGridDesktop]}>
        <Pressable onPress={onOpenAlerts} style={[styles.quickActionCard, desktop && styles.quickActionCardDesktop]}>
          <View style={styles.quickActionIcon}><Text style={styles.quickActionIconText}>✦</Text></View>
          <View style={styles.quickActionCopy}>
            <Text style={styles.quickActionTitle}>Alertas inteligentes</Text>
            <Text style={styles.quickActionText}>Revise canais e critérios do radar.</Text>
          </View>
          <Text style={styles.quickActionArrow}>→</Text>
        </Pressable>
        <Pressable onPress={onOpenActivity} style={[styles.quickActionCard, desktop && styles.quickActionCardDesktop]}>
          <View style={[styles.quickActionIcon, styles.quickActionIconDark]}><Text style={styles.quickActionIconText}>↗</Text></View>
          <View style={styles.quickActionCopy}>
            <Text style={styles.quickActionTitle}>Atividades</Text>
            <Text style={styles.quickActionText}>Acompanhe os últimos movimentos.</Text>
          </View>
          <Text style={styles.quickActionArrow}>→</Text>
        </Pressable>
      </View>

      <View style={styles.kpiGrid}>
        <KpiCard desktop={desktop} label="Pipeline ativo" value={String(activeDeals.length)} detail="oportunidades em andamento" tone="blue" />
        <KpiCard desktop={desktop} label="Valor potencial" value={money(pipelineValue)} detail="somado no funil" />
        <KpiCard desktop={desktop} label="Conversão" value={`${conversion}%`} detail={`${wonDeals.length} contratos ganhos`} tone="green" />
        <KpiCard desktop={desktop} label="Prazos próximos" value={String(urgent.length)} detail="até 7 dias" tone={urgent.length ? "orange" : "neutral"} />
      </View>

      <View style={[styles.insightGrid, desktop && styles.insightGridDesktop]}>
      <View style={[styles.insightPanel, desktop && styles.insightPanelDesktop]}>
        <View style={styles.sectionHeaderRow}>
          <View>
            <Text style={styles.sectionEyebrow}>DISTRIBUIÇÃO</Text>
            <Text style={styles.sectionTitle}>Funil comercial</Text>
          </View>
          <Pressable onPress={onOpenPipeline}><Text style={styles.sectionLink}>Detalhes →</Text></Pressable>
        </View>
        <View style={styles.funnelList}>
          {stageCounts.map((item) => (
            <View key={item.stage} style={styles.funnelRow}>
              <Text style={styles.funnelLabel}>{item.label}</Text>
              <View style={styles.funnelTrack}>
                <View style={[styles.funnelFill, { width: `${Math.max(item.count ? 14 : 0, Math.round((item.count / maxStageCount) * 100))}%` }]} />
              </View>
              <Text style={styles.funnelCount}>{item.count}</Text>
            </View>
          ))}
        </View>
      </View>

      <View style={[styles.insightPanel, desktop && styles.insightPanelDesktop]}>
        <View style={styles.sectionHeaderRow}>
          <View>
            <Text style={styles.sectionEyebrow}>PRIORIDADE</Text>
            <Text style={styles.sectionTitle}>Próximos prazos</Text>
          </View>
          <Pressable onPress={onOpenRadar}><Text style={styles.sectionLink}>Abrir radar →</Text></Pressable>
        </View>
        {urgent.length === 0 ? (
          <Text style={styles.sectionCopy}>Nenhuma licitação encerra nos próximos sete dias.</Text>
        ) : urgent.slice(0, 3).map((item) => (
          <View key={item.id} style={styles.deadlineInsightRow}>
            <View style={styles.deadlineInsightDate}><Text style={styles.deadlineInsightDateText}>{deadlineLabel(item.proposal_deadline)}</Text></View>
            <View style={styles.deadlineInsightCopy}>
              <Text style={styles.deadlineInsightTitle} numberOfLines={2}>{item.object}</Text>
              <Text style={styles.dealMeta}>{item.organization_name}</Text>
            </View>
          </View>
        ))}
      </View>
      </View>

      {usage && (
        <View style={styles.usageStrip}>
          <Text style={styles.usageStripTitle}>Uso de IA</Text>
          <Text style={styles.usageStripValue}>{usage.ai_analyses.used}/{usage.ai_analyses.limit}</Text>
          <View style={styles.usageStripTrack}>
            <View style={[styles.usageStripFill, { width: `${Math.min(100, Math.round((usage.ai_analyses.used / Math.max(1, usage.ai_analyses.limit)) * 100))}%` }]} />
          </View>
        </View>
      )}
    </ScrollView>
  );
}

function KpiCard({ desktop, label, value, detail, tone = "neutral" }: { desktop: boolean; label: string; value: string; detail: string; tone?: "blue" | "green" | "orange" | "neutral" }) {
  return (
    <View style={[styles.kpiCard, desktop && styles.kpiCardDesktop]}>
      <View style={[styles.kpiDot, tone === "blue" && styles.kpiDotBlue, tone === "green" && styles.kpiDotGreen, tone === "orange" && styles.kpiDotOrange]} />
      <Text style={styles.kpiLabel}>{label}</Text>
      <Text style={styles.kpiValue} numberOfLines={1}>{value}</Text>
      <Text style={styles.kpiDetail}>{detail}</Text>
    </View>
  );
}

function AlertsScreen({
  desktop,
  profile,
  preferences,
  opportunities,
  onOpenRadar,
  onOpenSettings,
}: {
  desktop: boolean;
  profile?: Profile;
  preferences: Preferences;
  opportunities: Opportunity[];
  onOpenRadar: () => void;
  onOpenSettings: () => void;
}) {
  const enabledChannels = [preferences.email, preferences.push, preferences.whatsapp].filter(Boolean).length;
  const terms = profile?.keywords ?? [];
  const urgent = opportunities.filter((item) => {
    const days = daysUntil(item.proposal_deadline);
    return days >= 0 && days <= 7;
  }).length;
  return (
    <ScrollView contentContainerStyle={[styles.pageScroll, desktop && styles.pageScrollDesktop]} showsVerticalScrollIndicator={false}>
      <View style={styles.screenIntro}>
        <Text style={styles.screenEyebrow}>AUTOMAÇÃO COMERCIAL</Text>
        <Text style={styles.screenTitle}>Seu radar trabalhando por você.</Text>
        <Text style={styles.screenCopy}>Os alertas usam seu perfil para destacar oportunidades aderentes e prazos que merecem ação.</Text>
      </View>

      <View style={styles.alertStatusHero}>
        <View style={styles.alertStatusOrb}><Text style={styles.alertStatusOrbText}>✦</Text></View>
        <View style={styles.alertStatusCopy}>
          <Text style={styles.alertStatusLabel}>{terms.length ? "RADAR CONFIGURADO" : "RADAR INCOMPLETO"}</Text>
          <Text style={styles.alertStatusTitle}>{terms.length ? "Pronto para encontrar oportunidades." : "Defina o foco da sua empresa."}</Text>
          <Text style={styles.alertStatusText}>{terms.length ? `${terms.length} termos e ${enabledChannels} canais ativos.` : "Adicione palavras-chave e regiões para começar."}</Text>
        </View>
      </View>

      <View style={styles.alertStatsRow}>
        <View style={styles.alertStat}><Text style={styles.alertStatValue}>{terms.length}</Text><Text style={styles.alertStatLabel}>termos ativos</Text></View>
        <View style={styles.alertStat}><Text style={styles.alertStatValue}>{enabledChannels}</Text><Text style={styles.alertStatLabel}>canais ligados</Text></View>
        <View style={styles.alertStat}><Text style={styles.alertStatValue}>{urgent}</Text><Text style={styles.alertStatLabel}>prazos próximos</Text></View>
      </View>

      <View style={[styles.desktopPanelGrid, desktop && styles.desktopPanelGridActive]}>
      <View style={[styles.settingsPanel, desktop && styles.desktopPanel]}>
        <View style={styles.sectionHeaderRow}>
          <View><Text style={styles.sectionEyebrow}>CRITÉRIOS</Text><Text style={styles.sectionTitle}>O que está sendo monitorado</Text></View>
          <Pressable onPress={onOpenRadar}><Text style={styles.sectionLink}>Editar →</Text></Pressable>
        </View>
        {terms.length ? (
          <View style={styles.termCloud}>{terms.map((term) => <View key={term} style={styles.termChip}><Text style={styles.termChipText}>{term}</Text></View>)}</View>
        ) : <Text style={styles.muted}>Nenhuma palavra-chave definida.</Text>}
        <View style={styles.alertFilterLine}><Text style={styles.alertFilterLabel}>Regiões</Text><Text style={styles.alertFilterValue}>{profile?.states?.length ? profile.states.join(" · ") : "Todo o Brasil"}</Text></View>
        <View style={styles.alertFilterLine}><Text style={styles.alertFilterLabel}>Faixa de valor</Text><Text style={styles.alertFilterValue}>{profile?.minimum_value_cents || profile?.maximum_value_cents ? `${money(profile.minimum_value_cents)} a ${money(profile.maximum_value_cents)}` : "Sem limite definido"}</Text></View>
      </View>

      <View style={[styles.settingsPanel, desktop && styles.desktopPanel]}>
        <Text style={styles.sectionEyebrow}>CANAIS</Text>
        <AlertChannelStatus label="E-mail" enabled={preferences.email} description="Resumo das oportunidades aderentes" />
        <AlertChannelStatus label="Notificações no app" enabled={preferences.push} description="Avisos enquanto você trabalha" />
        <AlertChannelStatus label="WhatsApp" enabled={preferences.whatsapp} description="Canal experimental" />
        <SecondaryButton label="Gerenciar preferências" onPress={onOpenSettings} />
      </View>
      </View>
    </ScrollView>
  );
}

function AlertChannelStatus({ label, enabled, description }: { label: string; enabled: boolean; description: string }) {
  return (
    <View style={styles.alertChannelStatus}>
      <View style={[styles.alertChannelDot, enabled && styles.alertChannelDotEnabled]} />
      <View style={styles.alertChannelStatusCopy}><Text style={styles.dealTitle}>{label}</Text><Text style={styles.muted}>{description}</Text></View>
      <Text style={[styles.alertChannelState, enabled && styles.alertChannelStateEnabled]}>{enabled ? "Ativo" : "Desligado"}</Text>
    </View>
  );
}

type ActivityItem = { id: string; icon: string; title: string; detail: string; date?: string; tone?: "blue" | "green" | "orange" };

function ActivityScreen({ desktop, deals, opportunities }: { desktop: boolean; deals: Deal[]; opportunities: Opportunity[] }) {
  const events = useMemo<ActivityItem[]>(() => {
    const opportunityEvents = opportunities.map((item) => ({ id: `opportunity-${item.id}`, icon: "✦", title: "Nova oportunidade no radar", detail: item.object, date: item.published_at || item.updated_at, tone: "blue" as const }));
    const dealEvents = deals.flatMap((deal) => {
      const entries: ActivityItem[] = [{ id: `deal-${deal.id}`, icon: "↗", title: "Negócio adicionado ao pipeline", detail: deal.title, date: deal.created_at, tone: "green" }];
      if (deal.last_follow_up_note) entries.push({ id: `followup-${deal.id}`, icon: "•", title: "Follow-up registrado", detail: deal.last_follow_up_note, date: deal.updated_at, tone: "orange" });
      return entries;
    });
    return [...opportunityEvents, ...dealEvents].filter((event) => event.date).sort((a, b) => new Date(b.date ?? 0).getTime() - new Date(a.date ?? 0).getTime());
  }, [deals, opportunities]);
  return (
    <ScrollView contentContainerStyle={[styles.pageScroll, desktop && styles.pageScrollNarrowDesktop]} showsVerticalScrollIndicator={false}>
      <View style={styles.screenIntro}>
        <Text style={styles.screenEyebrow}>TIMELINE</Text>
        <Text style={styles.screenTitle}>Tudo que mudou, em um só lugar.</Text>
        <Text style={styles.screenCopy}>Uma visão rápida das oportunidades e movimentos mais recentes da sua operação.</Text>
      </View>
      <View style={styles.activitySummary}><Text style={styles.activitySummaryValue}>{events.length}</Text><View><Text style={styles.activitySummaryTitle}>movimentos registrados</Text><Text style={styles.muted}>Radar e pipeline combinados</Text></View></View>
      <View style={styles.activityTimeline}>
        {events.length === 0 ? <Text style={styles.muted}>Ainda não há atividades para mostrar.</Text> : events.map((event, index) => (
          <View key={event.id} style={styles.activityItem}>
            <View style={styles.activityRail}>{index < events.length - 1 && <View style={styles.activityRailLine} />}<View style={[styles.activityIcon, event.tone === "green" && styles.activityIconGreen, event.tone === "orange" && styles.activityIconOrange]}><Text style={styles.activityIconText}>{event.icon}</Text></View></View>
            <View style={styles.activityCopy}><Text style={styles.activityTitle}>{event.title}</Text><Text style={styles.activityDetail} numberOfLines={2}>{event.detail}</Text><Text style={styles.activityDate}>{event.date ? formatWhen(event.date) : "Agora"}</Text></View>
          </View>
        ))}
      </View>
    </ScrollView>
  );
}

function DashboardScreen(props: {
  desktop: boolean;
  deals: Deal[];
  mode: "kanban" | "list";
  onModeChange: (mode: "kanban" | "list") => void;
  highlightFollowUpDealId?: string;
  loadFollowUps: (dealId: string) => Promise<DealFollowUp[]>;
  onOpenOpportunity: (deal: Deal) => void;
  onMove: (deal: Deal, stage: DealStage) => Promise<void>;
  onMarkLost: (deal: Deal) => Promise<void>;
  onFollowUp: (deal: Deal, note: string) => Promise<void>;
  onCreate: (title: string) => Promise<void>;
}) {
  const [title, setTitle] = useState("");
  const [composerOpen, setComposerOpen] = useState(false);
  const openStages: DealStage[] = ["prospecting", "analysis", "proposal", "negotiation"];
  const createDeal = () => {
    const nextTitle = title.trim();
    if (!nextTitle) return;
    void props.onCreate(nextTitle).then(() => {
      setTitle("");
      setComposerOpen(false);
    });
  };
  return (
    <View style={{ flex: 1 }}>
      <View style={[styles.toolbar, props.desktop && styles.toolbarDesktop]}>
        <View style={styles.segmentedControl}>
          <Pressable
            accessibilityRole="button"
            onPress={() => props.onModeChange("kanban")}
            style={[styles.segment, props.mode === "kanban" && styles.segmentActive]}
          >
            <Text style={[styles.segmentText, props.mode === "kanban" && styles.segmentTextActive]}>Kanban</Text>
          </Pressable>
          <Pressable
            accessibilityRole="button"
            onPress={() => props.onModeChange("list")}
            style={[styles.segment, props.mode === "list" && styles.segmentActive]}
          >
            <Text style={[styles.segmentText, props.mode === "list" && styles.segmentTextActive]}>Lista</Text>
          </Pressable>
        </View>
        <Pressable accessibilityRole="button" onPress={() => setComposerOpen(true)} style={styles.addButton}>
          <Text style={styles.addButtonText}>+ Nova</Text>
        </Pressable>
      </View>
      {props.mode === "kanban" ? (
        <ScrollView horizontal contentContainerStyle={[styles.kanban, props.desktop && styles.kanbanDesktop]}>
          {openStages.map((stage) => (
            <View key={stage} style={[styles.column, props.desktop && styles.columnDesktop]}>
              <View style={styles.columnHeader}>
                <Text style={styles.columnTitle}>{DEAL_STAGES.find((item) => item.key === stage)?.label}</Text>
                <Text style={styles.columnCount}>{props.deals.filter((deal) => deal.stage === stage).length}</Text>
              </View>
              {props.deals.filter((deal) => deal.stage === stage).map((deal) => (
                <DraggableDealCard
                  key={deal.id}
                  deal={deal}
                  stages={openStages}
                  onMove={props.onMove}
                >
                <DealCard
                  highlightFollowUp={props.highlightFollowUpDealId === deal.id}
                  deal={deal}
                  loadFollowUps={props.loadFollowUps}
                  onOpenOpportunity={deal.opportunity_id ? () => props.onOpenOpportunity(deal) : undefined}
                  onAdvance={async () => {
                  const idx = openStages.indexOf(stage);
                  const next: DealStage =
                    idx < 0 ? "prospecting" : idx >= openStages.length - 1 ? "won" : openStages[idx + 1]!;
                  await props.onMove(deal, next);
                }}
                  onMarkLost={() => props.onMarkLost(deal)}
                  onFollowUp={(note) => props.onFollowUp(deal, note)}
                />
                </DraggableDealCard>
              ))}
              {props.deals.filter((deal) => deal.stage === stage).length === 0 && (
                <View style={styles.columnEmpty}>
                  <Text style={styles.columnEmptyIcon}>+</Text>
                  <Text style={styles.columnEmptyText}>Solte um card aqui</Text>
                </View>
              )}
            </View>
          ))}
          <View style={[styles.column, props.desktop && styles.columnDesktop]}>
            <View style={styles.columnHeader}>
              <Text style={styles.columnTitle}>Fechados</Text>
              <Text style={styles.columnCount}>{props.deals.filter((deal) => deal.stage === "won" || deal.stage === "lost").length}</Text>
            </View>
            {props.deals.filter((deal) => deal.stage === "won" || deal.stage === "lost").map((deal) => (
              <DealCard
                key={deal.id}
                deal={deal}
                loadFollowUps={props.loadFollowUps}
                showHistory
                onOpenOpportunity={deal.opportunity_id ? () => props.onOpenOpportunity(deal) : undefined}
              />
            ))}
            {props.deals.filter((deal) => deal.stage === "won" || deal.stage === "lost").length === 0 && (
              <View style={styles.columnEmpty}><Text style={styles.columnEmptyText}>Nenhum negócio fechado</Text></View>
            )}
          </View>
        </ScrollView>
      ) : (
        <FlatList
          data={props.deals}
          keyExtractor={(item) => item.id}
          contentContainerStyle={[styles.pipelineList, props.desktop && styles.pipelineListDesktop]}
          ListEmptyComponent={<View style={styles.listEmpty}><Text style={styles.screenTitle}>Seu pipeline está vazio</Text><Text style={styles.screenCopy}>Crie uma oportunidade ou adicione uma licitação pelo radar.</Text></View>}
          renderItem={({ item }) => (
            <DealCard
              deal={item}
              expanded
              showHistory
              highlightFollowUp={props.highlightFollowUpDealId === item.id}
              loadFollowUps={props.loadFollowUps}
              onOpenOpportunity={item.opportunity_id ? () => props.onOpenOpportunity(item) : undefined}
              onMarkLost={item.stage !== "won" && item.stage !== "lost" ? () => props.onMarkLost(item) : undefined}
              onFollowUp={(note) => props.onFollowUp(item, note)}
            />
          )}
        />
      )}
      <Modal
        visible={composerOpen}
        transparent
        animationType="fade"
        onRequestClose={() => setComposerOpen(false)}
      >
        <View style={styles.composerOverlay}>
          <Pressable style={styles.composerBackdrop} onPress={() => setComposerOpen(false)} />
          <View style={styles.composerSheet}>
            <View style={styles.composerHandle} />
            <Text style={styles.composerEyebrow}>PIPELINE</Text>
            <Text style={styles.composerTitle}>Nova oportunidade</Text>
            <Text style={styles.composerCopy}>Crie um card para acompanhar uma negociação que ainda não veio do radar.</Text>
            <TextInput
              autoFocus
              value={title}
              onChangeText={setTitle}
              placeholder="Ex.: Renovação de contratos de TI"
              placeholderTextColor="#98A2B3"
              style={styles.input}
              onSubmitEditing={createDeal}
              returnKeyType="done"
            />
            <PrimaryButton label="Adicionar ao pipeline" onPress={createDeal} />
            <Pressable onPress={() => setComposerOpen(false)} style={styles.composerCancel}>
              <Text style={styles.composerCancelText}>Cancelar</Text>
            </Pressable>
          </View>
        </View>
      </Modal>
    </View>
  );
}

function DraggableDealCard({
  deal,
  stages,
  onMove,
  children,
}: {
  deal: Deal;
  stages: DealStage[];
  onMove: (deal: Deal, stage: DealStage) => Promise<void>;
  children: ReactNode;
}) {
  const position = useRef(new Animated.ValueXY()).current;
  const [dragging, setDragging] = useState(false);
  const stageIndex = stages.indexOf(deal.stage);

  const reset = useCallback(() => {
    Animated.spring(position, {
      toValue: { x: 0, y: 0 },
      useNativeDriver: false,
      tension: 90,
      friction: 9,
    }).start(() => setDragging(false));
  }, [position]);

  const panResponder = useMemo(() => PanResponder.create({
    onMoveShouldSetPanResponder: (_, gesture) => Math.abs(gesture.dx) > 8 && Math.abs(gesture.dx) > Math.abs(gesture.dy),
    onMoveShouldSetPanResponderCapture: (_, gesture) => Math.abs(gesture.dx) > 8 && Math.abs(gesture.dx) > Math.abs(gesture.dy),
    onPanResponderGrant: () => setDragging(true),
    onPanResponderMove: Animated.event([null, { dx: position.x, dy: position.y }], { useNativeDriver: false }),
    onPanResponderRelease: (_, gesture) => {
      const direction = gesture.dx > 72 ? 1 : gesture.dx < -72 ? -1 : 0;
      const nextStage = stages[stageIndex + direction];
      if (!direction || !nextStage) {
        reset();
        return;
      }
      Animated.timing(position, {
        toValue: { x: direction * 180, y: 0 },
        duration: 160,
        useNativeDriver: false,
      }).start(() => {
        position.setValue({ x: 0, y: 0 });
        setDragging(false);
        void onMove(deal, nextStage);
      });
    },
    onPanResponderTerminate: reset,
  }), [deal, onMove, position, reset, stageIndex, stages]);

  return (
    <Animated.View
      {...panResponder.panHandlers}
      style={[
        styles.draggableCard,
        dragging && styles.draggableCardActive,
        { transform: [...position.getTranslateTransform(), { rotate: position.x.interpolate({ inputRange: [-180, 0, 180], outputRange: ["-2deg", "0deg", "2deg"] }) }] },
      ]}
    >
      <View accessible accessibilityLabel={`Mover ${deal.title}`} accessibilityHint="Arraste horizontalmente para mudar de etapa" style={styles.dragHandle}>
        <View style={styles.dragHandleDot} />
        <View style={styles.dragHandleDot} />
        <View style={styles.dragHandleDot} />
        <Text style={styles.dragHint}>Arraste para mudar de etapa</Text>
      </View>
      {children}
    </Animated.View>
  );
}

function DealCard(props: {
  deal: Deal;
  expanded?: boolean;
  showHistory?: boolean;
  highlightFollowUp?: boolean;
  loadFollowUps?: (dealId: string) => Promise<DealFollowUp[]>;
  onOpenOpportunity?: () => void;
  onAdvance?: () => void;
  onMarkLost?: () => void;
  onFollowUp?: (note: string) => void;
}) {
  const [note, setNote] = useState("");
  const [historyOpen, setHistoryOpen] = useState(Boolean(props.showHistory));
  const [followUpOpen, setFollowUpOpen] = useState(Boolean(props.highlightFollowUp));
  const [history, setHistory] = useState<DealFollowUp[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);

  useEffect(() => {
    if (!historyOpen || !props.loadFollowUps) return;
    setHistoryLoading(true);
    void props
      .loadFollowUps(props.deal.id)
      .then(setHistory)
      .catch(() => setHistory([]))
      .finally(() => setHistoryLoading(false));
  }, [historyOpen, props.deal.id, props.loadFollowUps, props.deal.last_follow_up_note]);

  useEffect(() => {
    if (props.highlightFollowUp) setFollowUpOpen(true);
  }, [props.highlightFollowUp]);

  return (
    <View style={styles.dealCard}>
      <View style={styles.dealCardHeader}>
        <Text style={styles.dealTitle}>{props.deal.title}</Text>
        <Text style={styles.stageBadge}>{DEAL_STAGES.find((item) => item.key === props.deal.stage)?.label}</Text>
      </View>
      {props.deal.buyer_name ? <Text style={styles.dealMeta}>{props.deal.buyer_name}</Text> : null}
      {props.deal.last_follow_up_note ? <Text numberOfLines={2} style={styles.lastFollowUp}>“{props.deal.last_follow_up_note}”</Text> : null}
      {props.deal.opportunity_id ? <Text style={styles.linkedBadge}>• Vinculada ao radar</Text> : null}
      {props.onOpenOpportunity && (
        <Pressable onPress={props.onOpenOpportunity} style={styles.cardLink}>
          <Text style={styles.cardLinkText}>Ver licitação →</Text>
        </Pressable>
      )}
      <View style={styles.dealActions}>
        {props.onAdvance && <PrimaryButton label="Avançar" onPress={props.onAdvance} />}
        {props.onMarkLost && <DangerButton label="Perdida" onPress={props.onMarkLost} />}
      </View>
      {props.loadFollowUps && (
        <View style={styles.cardUtilityRow}>
          <Pressable onPress={() => setHistoryOpen((value) => !value)}>
            <Text style={styles.linkText}>{historyOpen ? "Ocultar histórico" : "Histórico"}</Text>
          </Pressable>
          {props.onFollowUp && !followUpOpen && (
            <Pressable onPress={() => setFollowUpOpen(true)}>
              <Text style={styles.linkText}>+ Follow-up</Text>
            </Pressable>
          )}
        </View>
      )}
      {historyOpen && (
        <View style={styles.historyBox}>
          {historyLoading && <Text style={styles.muted}>Carregando…</Text>}
          {!historyLoading && history.length === 0 && <Text style={styles.muted}>Nenhum follow-up registrado.</Text>}
          {history.map((entry) => (
            <View key={entry.id} style={styles.historyItem}>
              <Text style={styles.historyDate}>{formatWhen(entry.created_at)}</Text>
              <Text style={styles.historyNote}>{entry.note}</Text>
            </View>
          ))}
        </View>
      )}
      {props.onFollowUp && followUpOpen && (
        <View style={styles.followUpComposer}>
          <SpotlightRing active={Boolean(props.highlightFollowUp)}>
            <TextInput value={note} onChangeText={setNote} placeholder="Registrar follow-up" style={styles.input} />
          </SpotlightRing>
          <View style={styles.followUpActions}>
            <SecondaryButton
              label="Salvar follow-up"
              onPress={() => {
                if (note.trim()) {
                  props.onFollowUp?.(note.trim());
                  setNote("");
                  setHistoryOpen(true);
                  setFollowUpOpen(false);
                }
              }}
            />
            <Pressable onPress={() => setFollowUpOpen(false)} style={styles.followUpCancel}><Text style={styles.followUpCancelText}>Cancelar</Text></Pressable>
          </View>
        </View>
      )}
    </View>
  );
}

function formatWhen(value: string) {
  try {
    return new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" }).format(new Date(value));
  } catch {
    return value;
  }
}

function RadarScreen({
  desktop,
  opportunities,
  deals,
  profile,
  preferences,
  refreshing,
  onRefresh,
  onSelect,
  onAddToPipeline,
  onSaveAlert,
  highlightAlert,
  highlightFirstCard,
}: {
  desktop: boolean;
  opportunities: Opportunity[];
  deals: Deal[];
  profile?: Profile;
  preferences: Preferences;
  refreshing: boolean;
  onRefresh: () => void;
  onSelect: (item: Opportunity) => void;
  onAddToPipeline: (item: Opportunity) => Promise<void>;
  onSaveAlert: (input: { keywords: string[]; states: string[]; email: boolean }) => Promise<void>;
  highlightAlert?: boolean;
  highlightFirstCard?: boolean;
}) {
  const linked = useMemo(() => new Set(deals.map((deal) => deal.opportunity_id).filter(Boolean)), [deals]);
  const [alertOpen, setAlertOpen] = useState(false);
  const [alertKeywords, setAlertKeywords] = useState(profile?.keywords?.join(", ") ?? "");
  const [alertStates, setAlertStates] = useState(profile?.states?.join(", ") ?? "");
  const [alertEmail, setAlertEmail] = useState(preferences.email);
  const [alertSaving, setAlertSaving] = useState(false);
  const [alertError, setAlertError] = useState<string>();

  const openAlertBuilder = () => {
    setAlertKeywords(profile?.keywords?.join(", ") ?? "");
    setAlertStates(profile?.states?.join(", ") ?? "");
    setAlertEmail(preferences.email);
    setAlertError(undefined);
    setAlertOpen(true);
  };

  const saveAlert = async () => {
    const keywords = splitList(alertKeywords);
    if (!keywords.length) {
      setAlertError("Informe ao menos uma palavra-chave para o alerta.");
      return;
    }
    setAlertSaving(true);
    setAlertError(undefined);
    try {
      await onSaveAlert({ keywords, states: splitList(alertStates).map((state) => state.toUpperCase()), email: alertEmail });
      setAlertOpen(false);
    } catch (cause) {
      setAlertError(userFacingError(cause, "Não foi possível salvar o alerta."));
    } finally {
      setAlertSaving(false);
    }
  };

  return (
    <View style={{ flex: 1 }}>
    <FlatList
      key={desktop ? "desktop-grid" : "mobile-list"}
      numColumns={desktop ? 2 : 1}
      columnWrapperStyle={desktop ? styles.radarGridRow : undefined}
      data={opportunities}
      keyExtractor={(item) => item.id}
      contentContainerStyle={[styles.radarList, desktop && styles.radarListDesktop]}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} />}
      ListHeaderComponent={
        <>
          <View style={styles.screenIntro}>
            <Text style={styles.screenEyebrow}>RADAR</Text>
            <Text style={styles.screenTitle}>Oportunidades para avaliar</Text>
            <Text style={styles.screenCopy}>Abra uma licitação para ver prazo, valor e aderência ao seu perfil.</Text>
          </View>
          <ComplianceNotice compact />
          <SpotlightRing active={Boolean(highlightAlert)}>
          <View style={styles.alertCard}>
            <View style={styles.alertIcon}><Text style={styles.alertIconText}>✦</Text></View>
            <View style={styles.alertCardCopy}>
              <Text style={styles.alertCardTitle}>Alerta inteligente</Text>
              <Text style={styles.alertCardText} numberOfLines={2}>
                {profile?.keywords?.length ? `${profile.keywords.slice(0, 3).join(" · ")}${profile.keywords.length > 3 ? " +" : ""}` : "Defina temas e regiões para personalizar o radar."}
              </Text>
            </View>
            <Pressable onPress={openAlertBuilder} style={styles.alertConfigureButton}>
              <Text style={styles.alertConfigureText}>{profile?.keywords?.length ? "Editar" : "Criar"}</Text>
            </Pressable>
          </View>
          </SpotlightRing>
        </>
      }
      ListEmptyComponent={<Text style={styles.muted}>Nenhuma licitação no radar. Ajuste seu perfil comercial na aba Perfil.</Text>}
      renderItem={({ item, index }) => {
        const inPipeline = linked.has(item.id);
        const days = daysUntil(item.proposal_deadline);
        const card = (
          <View style={[styles.radarCard, desktop && styles.radarCardDesktop]}>
            <Pressable onPress={() => onSelect(item)}>
              <View style={styles.radarTop}>
                <View style={styles.radarBadges}>
                  <Text style={styles.badge}>{item.state || "BR"}</Text>
                  {item.source === "demo" && <Text style={styles.demoBadge}>Demo</Text>}
                </View>
                <Text style={[styles.deadline, days <= 3 && days >= 0 && styles.deadlineUrgent]}>{deadlineLabel(item.proposal_deadline)}</Text>
              </View>
              <Text style={styles.radarTitle}>{item.object}</Text>
              <Text style={styles.dealMeta}>{item.organization_name}</Text>
              <Text style={styles.radarValue}>{money(item.estimated_value_cents)}</Text>
            </Pressable>
            {inPipeline ? (
              <Text style={styles.inPipeline}>No pipeline</Text>
            ) : (
              <Pressable onPress={() => void onAddToPipeline(item)} style={styles.radarAction}>
                <Text style={styles.radarActionText}>Adicionar ao pipeline</Text>
              </Pressable>
            )}
          </View>
        );
        if (highlightFirstCard && index === 0) {
          return <SpotlightRing active>{card}</SpotlightRing>;
        }
        return card;
      }}
    />
    <Modal visible={alertOpen} transparent animationType="fade" onRequestClose={() => setAlertOpen(false)}>
      <View style={styles.composerOverlay}>
        <Pressable style={styles.composerBackdrop} onPress={() => setAlertOpen(false)} />
        <View style={styles.alertSheet}>
          <View style={styles.composerHandle} />
          <View style={styles.alertSheetHeader}>
            <View style={styles.alertIconLarge}><Text style={[styles.alertIconText, styles.alertIconTextLarge]}>✦</Text></View>
            <View style={styles.alertSheetHeaderCopy}>
              <Text style={styles.composerEyebrow}>ALERTA INTELIGENTE</Text>
              <Text style={styles.composerTitle}>O que você quer encontrar?</Text>
            </View>
          </View>
          <Text style={styles.composerCopy}>Usaremos seu perfil comercial e estas preferências para priorizar novas licitações.</Text>
          <Field label="Palavras-chave" value={alertKeywords} onChangeText={setAlertKeywords} />
          <Text style={styles.fieldHint}>Ex.: notebooks, servidores, suporte técnico</Text>
          <Field label="Estados atendidos" value={alertStates} onChangeText={setAlertStates} />
          <Text style={styles.fieldHint}>Use siglas separadas por vírgula. Deixe vazio para todo o Brasil.</Text>
          <View style={styles.alertChannelRow}>
            <View style={styles.alertChannelCopy}>
              <Text style={styles.alertChannelTitle}>Receber por e-mail</Text>
              <Text style={styles.alertChannelText}>Enviaremos um resumo quando houver oportunidades aderentes.</Text>
            </View>
            <Switch
              value={alertEmail}
              onValueChange={setAlertEmail}
              trackColor={{ false: "#D0D5DD", true: "#9DBAFD" }}
              thumbColor={alertEmail ? colors.blue : "#F2F4F7"}
            />
          </View>
          {alertError && <Text style={styles.errorInline}>{alertError}</Text>}
          <PrimaryButton label={alertSaving ? "Salvando alerta…" : "Salvar alerta"} disabled={alertSaving} onPress={() => void saveAlert()} />
          <Pressable onPress={() => setAlertOpen(false)} style={styles.composerCancel}>
            <Text style={styles.composerCancelText}>Cancelar</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
    </View>
  );
}

function OpportunityDetailModal(props: {
  item?: Opportunity;
  analysis?: Analysis;
  inPipeline: boolean;
  highlightAnalyze?: boolean;
  highlightPipeline?: boolean;
  onClose: () => void;
  onAnalyze: () => Promise<void>;
  onAddToPipeline: () => Promise<void>;
  onOpenSource: (url: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  useEffect(() => {
    setError(undefined);
    setBusy(false);
  }, [props.item]);
  if (!props.item) return null;
  const item = props.item;
  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={props.onClose}>
      <SafeAreaView style={styles.modalSafe}>
        <View style={styles.modalBar}>
          <View>
            <Text style={styles.modalEyebrow}>LICITAÇÃO</Text>
            <Text style={styles.modalBarTitle}>Detalhes da oportunidade</Text>
          </View>
          <Pressable onPress={props.onClose} style={styles.modalClose}><Text style={styles.modalBack}>Fechar</Text></Pressable>
        </View>
        <ScrollView contentContainerStyle={styles.modalContent}>
          <Text style={styles.badge}>{item.state || "BR"}</Text>
          <Text style={styles.modalTitle}>{item.object}</Text>
          <Text style={styles.muted}>{item.organization_name}</Text>
          <View style={styles.summaryRow}>
            <SummaryChip label="Valor" value={money(item.estimated_value_cents)} />
            <SummaryChip label="Prazo" value={deadlineLabel(item.proposal_deadline)} />
          </View>
          {!props.analysis && (
            <SpotlightRing active={Boolean(props.highlightAnalyze)}>
              <View style={styles.callout}>
                <Text style={styles.calloutTitle}>Analisar aderência</Text>
                <Text style={styles.calloutCopy}>Cruze esta licitação com seu perfil comercial antes de decidir.</Text>
                <PrimaryButton
                  label={busy ? "Analisando…" : "Analisar aderência"}
                  disabled={busy}
                  onPress={() => {
                    setBusy(true);
                    setError(undefined);
                    void props.onAnalyze().catch((cause) => setError(cause instanceof Error ? cause.message : "Análise indisponível.")).finally(() => setBusy(false));
                  }}
                />
              </View>
            </SpotlightRing>
          )}
          {props.analysis?.matched && props.analysis.match && (
            <View style={styles.analysisBox}>
              <Text style={styles.analysisScore}>{props.analysis.match.score ?? 0}/100 aderência ao perfil</Text>
              <Text style={styles.analysisDisclaimer}>Indicador comercial, não é avaliação de habilitação ou julgamento da licitação.</Text>
              <Text style={styles.muted}>{props.analysis.match.explanation || props.analysis.match.evidence?.join(" · ")}</Text>
            </View>
          )}
          {props.analysis && !props.analysis.matched && <Text style={styles.muted}>Fora dos filtros do perfil atual.</Text>}
          {error && <Text style={styles.errorInline}>{error}</Text>}
          {props.inPipeline ? (
            <Text style={styles.inPipeline}>Esta licitação já está no pipeline.</Text>
          ) : (
            <SpotlightRing active={Boolean(props.highlightPipeline)}>
              <PrimaryButton label="Adicionar ao pipeline" onPress={() => void props.onAddToPipeline()} />
            </SpotlightRing>
          )}
          <ComplianceNotice />
          <View style={styles.sourceCard}>
            <Text style={styles.metaLabel}>{item.source === "demo" ? "DADOS DE DEMONSTRAÇÃO" : "FONTE E ATUALIZAÇÃO"}</Text>
            <Text style={styles.sourceMeta}>{opportunitySourceLabel(item.source)} · {opportunityFreshness(item.updated_at, item.published_at)}</Text>
            {item.published_at ? <Text style={styles.sourceMeta}>Publicação: {new Date(item.published_at).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" })}</Text> : null}
            {item.source === "demo" && <Text style={styles.sourceWarning}>Este registro é ilustrativo e não representa um edital oficial.</Text>}
          </View>
          {item.source_url ? (
            <SecondaryButton label="Abrir fonte oficial" onPress={() => props.onOpenSource(item.source_url!)} />
          ) : null}
        </ScrollView>
      </SafeAreaView>
    </Modal>
  );
}

function ProfileEditorScreen({ profile, onSave, highlightSave }: { profile: Profile; onSave: (input: ProfileInput) => Promise<Profile>; highlightSave?: boolean }) {
  const [draft, setDraft] = useState(() => profileToDraft(profile));
  const [message, setMessage] = useState<string>();
  const [saving, setSaving] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  return (
    <ScrollView contentContainerStyle={styles.pageScroll} keyboardShouldPersistTaps="handled">
      <View style={styles.screenIntro}>
        <Text style={styles.screenEyebrow}>PERFIL COMERCIAL</Text>
        <Text style={styles.screenTitle}>Aprimore seu radar</Text>
        <Text style={styles.screenCopy}>Quanto mais preciso o perfil, melhores serão as licitações e análises sugeridas.</Text>
      </View>
      <View style={styles.formPanel}>
        <Text style={styles.formSectionLabel}>IDENTIDADE DO PERFIL</Text>
        <Field label="Nome do perfil" value={draft.name} onChangeText={(name) => setDraft({ ...draft, name })} />
        <Field label="O que sua empresa fornece" value={draft.description} onChangeText={(description) => setDraft({ ...draft, description })} multiline />
      </View>
      <View style={styles.formPanel}>
        <Text style={styles.formSectionLabel}>SEGMENTAÇÃO DO RADAR</Text>
        <Field label="Palavras-chave" value={draft.keywords} onChangeText={(keywords) => setDraft({ ...draft, keywords })} />
        <Text style={styles.fieldHint}>Separe os termos por vírgula.</Text>
        <Field label="Estados atendidos" value={draft.states} onChangeText={(states) => setDraft({ ...draft, states })} />
        <Text style={styles.fieldHint}>Use as siglas, como PR, SC e SP.</Text>
        <Field label="Termos que devem ser ignorados" value={draft.excludedTerms} onChangeText={(excludedTerms) => setDraft({ ...draft, excludedTerms })} />
        <Text style={styles.fieldHint}>Ex.: usado, locação, seminovo</Text>
        <View style={styles.valueFieldsRow}>
          <View style={styles.valueField}>
            <Field label="Valor mínimo (R$)" value={draft.minimumValue} onChangeText={(minimumValue) => setDraft({ ...draft, minimumValue })} />
          </View>
          <View style={styles.valueField}>
            <Field label="Valor máximo (R$)" value={draft.maximumValue} onChangeText={(maximumValue) => setDraft({ ...draft, maximumValue })} />
          </View>
        </View>
      </View>
      <Pressable onPress={() => setAdvancedOpen((current) => !current)} style={styles.advancedToggle}>
        <View>
          <Text style={styles.advancedToggleTitle}>Filtros avançados</Text>
          <Text style={styles.advancedToggleCopy}>Categorias, municípios e modalidades</Text>
        </View>
        <Text style={styles.advancedToggleIcon}>{advancedOpen ? "−" : "+"}</Text>
      </Pressable>
      {advancedOpen && (
        <View style={styles.formPanel}>
          <Text style={styles.formSectionLabel}>FILTROS AVANÇADOS</Text>
          <Field label="Termos obrigatórios" value={draft.requiredTerms} onChangeText={(requiredTerms) => setDraft({ ...draft, requiredTerms })} />
          <Text style={styles.fieldHint}>Todos os termos informados devem aparecer.</Text>
          <Field label="Categorias" value={draft.categories} onChangeText={(categories) => setDraft({ ...draft, categories })} />
          <Field label="Municípios" value={draft.municipalities} onChangeText={(municipalities) => setDraft({ ...draft, municipalities })} />
          <Field label="Códigos de modalidade" value={draft.modalities} onChangeText={(modalities) => setDraft({ ...draft, modalities })} />
        </View>
      )}
      <View style={styles.profilePreview}>
        <View style={styles.profilePreviewIcon}><Text style={styles.profilePreviewIconText}>✦</Text></View>
        <View style={styles.profilePreviewCopy}>
          <Text style={styles.profilePreviewTitle}>Como o radar entende sua empresa</Text>
          <Text style={styles.profilePreviewText} numberOfLines={3}>
            {draft.description.trim() || "Descreva seus produtos e serviços para visualizar o foco do radar."}
          </Text>
        </View>
      </View>
      {message && <Text style={message.includes("sucesso") ? styles.successText : styles.errorInline}>{message}</Text>}
      <SpotlightRing active={Boolean(highlightSave)}>
        <PrimaryButton
          label={saving ? "Salvando…" : "Salvar perfil"}
          disabled={saving}
        onPress={() => {
          setSaving(true);
          setMessage(undefined);
          const input = draftToProfileInput(draft);
          if (!input.name || !input.description) {
            setMessage("Preencha nome e descrição do perfil.");
            setSaving(false);
            return;
          }
          void onSave(input)
            .then(() => setMessage("Perfil atualizado com sucesso."))
            .catch((cause) => setMessage(userFacingError(cause, "Não foi possível salvar.")))
            .finally(() => setSaving(false));
        }}
        />
      </SpotlightRing>
    </ScrollView>
  );
}

function PlanScreen({
  usage,
  organizationName,
  history,
  onCheckout,
  onPortal,
}: {
  usage: Usage;
  organizationName: string;
  history: SubscriptionHistoryEntry[];
  onCheckout: (plan: "essential" | "pro") => Promise<void>;
  onPortal: () => Promise<void>;
}) {
  return (
    <ScrollView contentContainerStyle={styles.pageScroll}>
      <View style={styles.planHero}>
        <Text style={styles.planEyebrow}>PLANO ATUAL</Text>
        <Text style={styles.planName}>{usage.plan === "pro" ? "Pro" : "Essencial"}</Text>
        <Text style={styles.planOrganization}>{organizationName}</Text>
      </View>
      <View style={styles.usageGrid}>
        <UsageMetric label="Perfis" value={`${usage.profiles.used}/${usage.profiles.limit}`} />
        <UsageMetric label="Análises IA" value={`${usage.ai_analyses.used}/${usage.ai_analyses.limit}`} />
        <UsageMetric label="Alertas" value={`${usage.daily_alerts.used}/${usage.daily_alerts.limit}`} />
      </View>
      <View style={styles.sectionHeader}>
        <Text style={styles.sectionTitle}>Escolha o que faz sentido</Text>
        <Text style={styles.sectionCopy}>Você pode alterar ou gerenciar sua assinatura a qualquer momento.</Text>
      </View>
      <PrimaryButton label="Fazer upgrade para Pro" onPress={() => onCheckout("pro")} />
      <SecondaryButton label="Ver plano Essencial" onPress={() => onCheckout("essential")} />
      <Pressable onPress={() => void onPortal()} style={styles.textAction}><Text style={styles.textActionLabel}>Gerenciar cobrança</Text></Pressable>
      <Text style={styles.sectionTitle}>Histórico</Text>
      {history.length === 0 ? (
        <Text style={styles.muted}>Nenhum evento registrado ainda.</Text>
      ) : (
        history.map((entry) => (
          <View key={entry.id} style={styles.historyItem}>
            <Text style={styles.dealTitle}>{entry.plan.toUpperCase()} · {entry.status}</Text>
            <Text style={styles.muted}>{formatWhen(entry.recorded_at)}</Text>
          </View>
        ))
      )}
    </ScrollView>
  );
}

function SettingsScreen({
  accountName,
  emailHint,
  preferences,
  onChange,
  onSignOut,
  onOpenGuide,
  onOpenPlan,
  onOpenLegal,
  highlightEmailSwitch,
}: {
  accountName: string;
  emailHint: string;
  preferences: Preferences;
  onChange: (value: Preferences) => Promise<void>;
  onSignOut: () => Promise<void>;
  onOpenGuide: () => void;
  onOpenPlan: () => void;
  onOpenLegal: (doc: LegalDoc) => void;
  highlightEmailSwitch?: boolean;
}) {
  const alertOptions: Array<[keyof Preferences, string, string]> = [
    ["push", "Notificações no app", "Novas oportunidades com boa aderência ao perfil"],
    ["email", "Resumo por e-mail", "Seleção diária das licitações do radar"],
    ["whatsapp", "WhatsApp", "Canal experimental (conexão em breve)"],
    ["deadlineReminder", "Lembrete de prazo", "Aviso antes do encerramento da proposta"],
  ];
  return (
    <ScrollView contentContainerStyle={styles.pageScroll}>
      <View style={styles.accountPanel}>
        <View style={styles.accountAvatar}><Text style={styles.accountAvatarText}>{(accountName || "U").trim().charAt(0).toUpperCase()}</Text></View>
        <View style={styles.accountCopy}>
          <Text style={styles.screenEyebrow}>CONTA</Text>
          <Text style={styles.accountName}>{accountName || "Usuário"}</Text>
          <Text style={styles.accountEmail}>{emailHint}</Text>
        </View>
      </View>
      <View style={styles.settingsPanel}>
        <Text style={styles.sectionTitle}>Alertas</Text>
        <Text style={styles.sectionCopy}>Defina como quer receber oportunidades e lembretes de prazo.</Text>
        {alertOptions.map(([key, title, copy], index) => {
          const row = (
            <View style={[styles.settingRow, index > 0 && styles.settingBorder]}>
              <View style={styles.settingCopy}>
                <Text style={styles.dealTitle}>{title}</Text>
                <Text style={styles.muted}>{copy}</Text>
              </View>
              <Switch
                value={preferences[key]}
                onValueChange={(value) => void onChange({ ...preferences, [key]: value })}
                trackColor={{ false: "#D0D5DD", true: "#9DBAFD" }}
                thumbColor={preferences[key] ? colors.blue : "#F2F4F7"}
              />
            </View>
          );
          if (key === "email" && highlightEmailSwitch) {
            return (
              <SpotlightRing key={key} active>
                {row}
              </SpotlightRing>
            );
          }
          return <View key={key}>{row}</View>;
        })}
      </View>
      <View style={styles.settingsPanel}>
        <Text style={styles.sectionTitle}>Precisa de ajuda?</Text>
        <Text style={styles.sectionCopy}>Revise o fluxo completo e avance no seu ritmo.</Text>
        <SecondaryButton label="Abrir roteiro guiado" onPress={onOpenGuide} />
      </View>
      <View style={styles.settingsPanel}>
        <Text style={styles.sectionTitle}>Legal e privacidade</Text>
        <Text style={styles.sectionCopy}>Termos, LGPD e versão dos documentos (v{LEGAL_VERSION}).</Text>
        <SecondaryButton label="Termos de Uso" onPress={() => onOpenLegal("terms")} />
        <SecondaryButton label="Política de Privacidade" onPress={() => onOpenLegal("privacy")} />
      </View>
      <Pressable onPress={onOpenPlan} style={styles.planShortcut}>
        <View style={styles.planShortcutCopy}>
          <Text style={styles.sectionTitle}>Plano e cobrança</Text>
          <Text style={styles.sectionCopy}>Uso, assinatura e histórico</Text>
        </View>
        <Text style={styles.planShortcutArrow}>→</Text>
      </Pressable>
      <Pressable onPress={() => void onSignOut()} style={styles.signOutAction}><Text style={styles.signOutText}>Sair da conta</Text></Pressable>
    </ScrollView>
  );
}

function SignUpScreen({
  onBack,
  onComplete,
  onOpenLegal,
}: {
  onBack: () => void;
  onComplete: (session: SessionPayload) => Promise<void>;
  onOpenLegal: (doc: LegalDoc) => void;
}) {
  const [form, setForm] = useState({ email: "", password: "", full_name: "", organization_name: "", plan: "essential" as const });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [acceptedLegal, setAcceptedLegal] = useState(false);
  return (
    <AuthShell title="Criar conta" copy="Cadastre sua empresa e comece o trial.">
      <Field label="Nome completo" value={form.full_name} onChangeText={(full_name) => setForm({ ...form, full_name })} />
      <Field label="E-mail corporativo" value={form.email} onChangeText={(email) => setForm({ ...form, email })} />
      <Field label="Senha (mín. 8)" value={form.password} onChangeText={(password) => setForm({ ...form, password })} secure />
      <Field label="Nome da empresa" value={form.organization_name} onChangeText={(organization_name) => setForm({ ...form, organization_name })} />
      <View style={styles.legalAcceptRow}>
        <Switch
          value={acceptedLegal}
          onValueChange={setAcceptedLegal}
          trackColor={{ false: "#D0D5DD", true: "#9DBAFD" }}
          thumbColor={acceptedLegal ? colors.blue : "#F2F4F7"}
        />
        <Text style={styles.legalAcceptText}>
          Li e aceito os{" "}
          <Text style={styles.legalLink} onPress={() => onOpenLegal("terms")}>Termos de Uso</Text>
          {" "}e a{" "}
          <Text style={styles.legalLink} onPress={() => onOpenLegal("privacy")}>Política de Privacidade</Text>.
        </Text>
      </View>
      <PrimaryButton label={busy ? "Criando…" : "Criar conta e continuar"} disabled={busy || !acceptedLegal} onPress={async () => {
        setBusy(true); setError(undefined);
        const email = form.email.trim();
        const organization_name = form.organization_name.trim();
        const full_name = form.full_name.trim();
        if (!full_name || !email || !organization_name) {
          setError("Preencha nome, e-mail e nome da empresa.");
          setBusy(false);
          return;
        }
        if (!acceptedLegal) {
          setError("Aceite os Termos e a Política de Privacidade para continuar.");
          setBusy(false);
          return;
        }
        if (form.password.length < 8) {
          setError("Senha deve ter ao menos 8 caracteres.");
          setBusy(false);
          return;
        }
        try {
          const anon = new LicitaLensClient({ baseUrl: apiUrl, organizationId: "" });
          await onComplete(await anon.signup({ ...form, email, full_name, organization_name }));
        } catch (cause) {
          setError(userFacingError(cause, "Falha no cadastro"));
        } finally { setBusy(false); }
      }} />
      {error && <Text style={styles.error}>{error}</Text>}
      <SecondaryButton label="Voltar" onPress={onBack} />
    </AuthShell>
  );
}

function LoginScreen({ onBack, onComplete }: { onBack: () => void; onComplete: (session: SessionPayload) => Promise<void> }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  return (
    <AuthShell title="Bem-vindo de volta" copy="Entre para acompanhar oportunidades, prazos e negociações em andamento.">
      <Field label="E-mail" placeholder="voce@empresa.com.br" value={email} onChangeText={setEmail} />
      <Field label="Senha" placeholder="Digite sua senha" value={password} onChangeText={setPassword} secure />
      <View style={styles.authSecureRow}>
        <View style={styles.authSecureIcon}><Text style={styles.authSecureIconText}>✓</Text></View>
        <Text style={styles.authSecureText}>Acesso protegido e exclusivo para sua organização</Text>
      </View>
      <PrimaryButton label={busy ? "Entrando…" : "Entrar"} disabled={busy} onPress={async () => {
        setBusy(true); setError(undefined);
        const trimmedEmail = email.trim();
        if (!trimmedEmail || !password) {
          setError("Informe e-mail e senha.");
          setBusy(false);
          return;
        }
        try {
          const anon = new LicitaLensClient({ baseUrl: apiUrl, organizationId: "" });
          await onComplete(await anon.login({ email: trimmedEmail, password }));
        } catch (cause) {
          setError(userFacingError(cause, "Falha no login"));
        } finally { setBusy(false); }
      }} />
      {error && <Text style={styles.error}>{error}</Text>}
      <View style={styles.authDivider}><View style={styles.authDividerLine} /><Text style={styles.authDividerText}>AINDA NÃO USA O LICITALENS?</Text><View style={styles.authDividerLine} /></View>
      <SecondaryButton label="Criar minha conta" onPress={onBack} />
    </AuthShell>
  );
}

function ProfileOnboarding({
  organizationName,
  onComplete,
  highlightStart,
}: {
  organizationName: string;
  onComplete: (input: ProfileInput) => Promise<void>;
  highlightStart?: boolean;
}) {
  const [name, setName] = useState("Radar principal");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    const trimmedName = name.trim();
    const trimmedDescription = description.trim();
    if (!trimmedName || !trimmedDescription) {
      setError("Preencha o nome do perfil e a descrição do que sua empresa fornece.");
      return;
    }
    setBusy(true);
    setError(undefined);
    try {
      await onComplete({
        name: trimmedName,
        description: trimmedDescription,
        keywords: [],
        states: [],
      });
    } catch (cause) {
      setError(userFacingError(cause, "Não foi possível salvar o perfil."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthShell title="Perfil comercial" copy={`Configure o que ${organizationName} fornece para alimentar licitações e alertas.`}>
      <Field label="Nome do perfil" value={name} onChangeText={setName} />
      <Field label="Descrição (obrigatório)" value={description} onChangeText={setDescription} multiline />
      <Text style={styles.muted}>Ex.: notebooks, serviços de TI e suporte para órgãos públicos.</Text>
      {error && <Text style={styles.errorInline}>{error}</Text>}
      <SpotlightRing active={Boolean(highlightStart)}>
        <PrimaryButton label={busy ? "Salvando…" : "Ir para o painel"} disabled={busy} onPress={() => void submit()} />
      </SpotlightRing>
    </AuthShell>
  );
}

function AuthShell({ title, copy, children }: { title: string; copy: string; children: React.ReactNode }) {
  const desktop = useWindowDimensions().width >= 900;
  return (
    <SafeAreaView style={[styles.auth, desktop && styles.authDesktop]}>
      <ScrollView contentContainerStyle={[styles.authScroll, desktop && styles.authScrollDesktop]} keyboardShouldPersistTaps="handled">
        <View style={desktop ? styles.authLayoutDesktop : styles.authLayout}>
          {desktop && (
            <View style={styles.authVisual}>
              <View style={styles.authVisualGlow} />
              <View style={styles.authBrandDesktop}>
                <View style={[styles.brandMark, styles.brandMarkDesktop]}><Text style={styles.brandMarkText}>L</Text></View>
                <View><Text style={styles.brandNameDesktop}>LicitaLens</Text><Text style={styles.brandCaptionDesktop}>Inteligência comercial pública</Text></View>
              </View>
              <View style={styles.authVisualContent}>
                <Text style={styles.authVisualEyebrow}>DO RADAR AO CONTRATO</Text>
                <Text style={styles.authVisualTitle}>Licitações certas.{"\n"}Decisões mais rápidas.</Text>
                <Text style={styles.authVisualCopy}>Descubra oportunidades aderentes ao seu negócio e conduza cada negociação com clareza.</Text>
                <View style={styles.authBenefits}>
                  <AuthBenefit icon="⌕" title="Radar inteligente" copy="Oportunidades priorizadas pelo seu perfil." />
                  <AuthBenefit icon="▦" title="Pipeline visual" copy="Prazos, etapas e follow-ups em um só fluxo." />
                  <AuthBenefit icon="✦" title="Alertas objetivos" copy="Aja no momento certo, pelo canal certo." />
                </View>
              </View>
              <View style={styles.authTrustPill}><View style={styles.authTrustDot} /><Text style={styles.authTrustText}>Operação conectada aos dados do PNCP</Text></View>
            </View>
          )}
          <View style={[styles.authFormColumn, desktop && styles.authFormColumnDesktop]}>
            <View style={[styles.authFormInner, desktop && styles.authFormInnerDesktop]}>
              {!desktop && (
                <View style={styles.authBrand}>
                  <View style={styles.brandMark}><Text style={styles.brandMarkText}>L</Text></View>
                  <Text style={styles.brandName}>LicitaLens</Text>
                </View>
              )}
              <View style={styles.authHero}>
                <Text style={styles.authEyebrow}>INTELIGÊNCIA COMERCIAL</Text>
                <Text style={[styles.authTitle, desktop && styles.authTitleDesktop]}>{title}</Text>
                <Text style={styles.authCopy}>{copy}</Text>
              </View>
              <View style={[styles.authPanel, desktop && styles.authPanelDesktop]}>
                {children}
              </View>
              <Text style={styles.authFooter}>LicitaLens · Experiência unificada no web e no celular</Text>
            </View>
          </View>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

function AuthBenefit({ icon, title, copy }: { icon: string; title: string; copy: string }) {
  return (
    <View style={styles.authBenefit}>
      <View style={styles.authBenefitIcon}><Text style={styles.authBenefitIconText}>{icon}</Text></View>
      <View style={styles.authBenefitCopy}><Text style={styles.authBenefitTitle}>{title}</Text><Text style={styles.authBenefitText}>{copy}</Text></View>
    </View>
  );
}

function DesktopSidebar({
  active,
  accountName,
  organizationName,
  subscriptionStatus,
  onChange,
}: {
  active: Tab;
  accountName: string;
  organizationName: string;
  subscriptionStatus: string;
  onChange: (tab: Tab) => void;
}) {
  return (
    <View style={styles.sidebar}>
      <View style={styles.sidebarBrand}>
        <View style={styles.sidebarBrandMark}><Text style={styles.sidebarBrandMarkText}>L</Text></View>
        <View><Text style={styles.sidebarBrandName}>LicitaLens</Text><Text style={styles.sidebarBrandTagline}>Inteligência comercial</Text></View>
      </View>
      <View style={styles.sidebarOrganization}>
        <Text style={styles.sidebarOrganizationLabel}>ESPAÇO DE TRABALHO</Text>
        <Text style={styles.sidebarOrganizationName} numberOfLines={1}>{organizationName || "Minha empresa"}</Text>
      </View>
      <View style={styles.sidebarNav}>
        {NAV_ITEMS.map((item) => (
          <Pressable
            accessibilityRole="button"
            accessibilityState={{ selected: active === item.key }}
            key={item.key}
            onPress={() => onChange(item.key)}
            style={({ pressed }) => [styles.sidebarItem, active === item.key && styles.sidebarItemActive, pressed && styles.sidebarItemPressed]}
          >
            <View style={[styles.sidebarIcon, active === item.key && styles.sidebarIconActive]}><Text style={[styles.sidebarIconText, active === item.key && styles.sidebarIconTextActive]}>{item.icon}</Text></View>
            <View style={styles.sidebarItemCopy}><Text style={[styles.sidebarItemLabel, active === item.key && styles.sidebarItemLabelActive]}>{item.label}</Text><Text style={styles.sidebarItemDescription}>{item.description}</Text></View>
          </Pressable>
        ))}
      </View>
      <Pressable onPress={() => onChange("plan")} style={[styles.sidebarPlan, active === "plan" && styles.sidebarPlanActive]}>
        <View style={styles.sidebarPlanTop}><Text style={styles.sidebarPlanEyebrow}>PLANO ATUAL</Text><Text style={styles.sidebarPlanArrow}>→</Text></View>
        <Text style={styles.sidebarPlanName}>{subscriptionStatus === "trialing" ? "Período de teste" : "LicitaLens Pro"}</Text>
        <Text style={styles.sidebarPlanCopy}>Ver uso e assinatura</Text>
      </Pressable>
      <View style={styles.sidebarAccount}>
        <View style={styles.sidebarAvatar}><Text style={styles.sidebarAvatarText}>{accountName.slice(0, 1).toUpperCase() || "U"}</Text></View>
        <View style={styles.sidebarAccountCopy}><Text style={styles.sidebarAccountName} numberOfLines={1}>{accountName || "Usuário"}</Text><Text style={styles.sidebarAccountRole}>Conta da organização</Text></View>
      </View>
    </View>
  );
}

function BottomNav({ active, onChange, spotlightTab }: { active: Tab; onChange: (tab: Tab) => void; spotlightTab?: Tab | null }) {
  return (
    <ScrollView horizontal style={styles.navScroll} showsHorizontalScrollIndicator={false} contentContainerStyle={styles.nav}>
      {NAV_ITEMS.map(({ key, icon, label }) => (
        <Pressable accessibilityRole="button" accessibilityState={{ selected: active === key }} key={key} style={[styles.navItem, active === key && styles.navItemActive, spotlightTab === key && styles.navSpotlight]} onPress={() => onChange(key)}>
          <Text style={[styles.navIcon, active === key && styles.navActive, spotlightTab === key && styles.navSpotlightText]}>{icon}</Text>
          <Text style={[styles.navLabel, active === key && styles.navActive, spotlightTab === key && styles.navSpotlightText]}>{label}</Text>
        </Pressable>
      ))}
    </ScrollView>
  );
}

function SummaryChip({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.summaryChip}>
      <Text style={styles.historyDate}>{label.toUpperCase()}</Text>
      <Text style={styles.dealTitle}>{value}</Text>
    </View>
  );
}

function UsageMetric({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.usageMetric}>
      <Text style={styles.usageMetricValue}>{value}</Text>
      <Text style={styles.usageMetricLabel}>{label}</Text>
    </View>
  );
}

type ProfileDraft = {
  name: string;
  description: string;
  keywords: string;
  states: string;
  categories: string;
  municipalities: string;
  modalities: string;
  requiredTerms: string;
  excludedTerms: string;
  minimumValue: string;
  maximumValue: string;
};

function profileToDraft(profile: Profile): ProfileDraft {
  return {
    name: profile.name,
    description: profile.description,
    keywords: profile.keywords?.join(", ") ?? "",
    states: profile.states?.join(", ") ?? "",
    categories: profile.categories?.join(", ") ?? "",
    municipalities: profile.municipalities?.join(", ") ?? "",
    modalities: profile.modalities?.join(", ") ?? "",
    requiredTerms: profile.required_terms?.join(", ") ?? "",
    excludedTerms: profile.excluded_terms?.join(", ") ?? "",
    minimumValue: profile.minimum_value_cents ? String(profile.minimum_value_cents / 100) : "",
    maximumValue: profile.maximum_value_cents ? String(profile.maximum_value_cents / 100) : "",
  };
}

function draftToProfileInput(draft: ProfileDraft): ProfileInput {
  return {
    name: draft.name.trim(),
    description: draft.description.trim(),
    keywords: splitList(draft.keywords),
    categories: splitList(draft.categories),
    states: splitList(draft.states).map((value) => value.toUpperCase()),
    municipalities: splitList(draft.municipalities),
    modalities: splitList(draft.modalities).map(Number).filter(Number.isFinite),
    required_terms: splitList(draft.requiredTerms),
    excluded_terms: splitList(draft.excludedTerms),
    minimum_value_cents: centsFromDraft(draft.minimumValue),
    maximum_value_cents: centsFromDraft(draft.maximumValue),
  };
}

function centsFromDraft(value: string) {
  if (!value.trim()) return undefined;
  const normalized = value.trim().replace(/\./g, "").replace(",", ".").replace(/[^\d.-]/g, "");
  const amount = Number(normalized);
  return Number.isFinite(amount) ? Math.round(amount * 100) : undefined;
}

function splitList(value: string) {
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

function money(cents?: number) {
  return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL", maximumFractionDigits: 0 }).format((cents ?? 0) / 100);
}

function daysUntil(value?: string) {
  if (!value) return 0;
  return Math.ceil((new Date(value).getTime() - Date.now()) / 86_400_000);
}

function deadlineLabel(value?: string) {
  if (!value) return "Prazo não informado";
  const days = daysUntil(value);
  return days < 0 ? "Encerrada" : days === 0 ? "Encerra hoje" : `${days} dias`;
}

function Field(props: { label: string; value: string; onChangeText: (value: string) => void; multiline?: boolean; secure?: boolean; placeholder?: string }) {
  const email = props.label.toLowerCase().includes("e-mail");
  return (
    <View style={{ gap: 6 }}>
      <Text style={styles.fieldLabel}>{props.label}</Text>
      <TextInput accessibilityLabel={props.label} autoCapitalize={email ? "none" : "sentences"} autoCorrect={!email && !props.secure} keyboardType={email ? "email-address" : "default"} placeholder={props.placeholder} placeholderTextColor="#98A2B3" value={props.value} onChangeText={props.onChangeText} multiline={props.multiline} secureTextEntry={props.secure} style={[styles.input, props.multiline && { minHeight: 90 }]} />
    </View>
  );
}

function PrimaryButton({ label, onPress, disabled }: { label: string; onPress: () => void; disabled?: boolean }) {
  return <Pressable accessibilityRole="button" accessibilityState={{ disabled: Boolean(disabled) }} onPress={onPress} disabled={disabled} style={({ pressed }) => [styles.primaryBtn, pressed && styles.buttonPressed, disabled && { opacity: 0.5 }]}><Text style={styles.primaryText}>{label}</Text></Pressable>;
}

function SecondaryButton({ label, onPress }: { label: string; onPress: () => void | Promise<void> }) {
  return <Pressable accessibilityRole="button" onPress={() => void onPress()} style={({ pressed }) => [styles.secondaryBtn, pressed && styles.buttonPressed]}><Text style={styles.secondaryText}>{label}</Text></Pressable>;
}

function DangerButton({ label, onPress }: { label: string; onPress: () => void | Promise<void> }) {
  return <Pressable accessibilityRole="button" onPress={() => void onPress()} style={styles.dangerBtn}><Text style={styles.dangerText}>{label}</Text></Pressable>;
}

const styles = StyleSheet.create({
  app: { flex: 1, backgroundColor: colors.canvas },
  appShell: { flex: 1, flexDirection: "row" },
  workspace: { flex: 1, minWidth: 0, backgroundColor: colors.canvas },
  workspaceContent: { flex: 1 },
  sidebar: { width: 272, backgroundColor: "#0D2341", paddingHorizontal: 16, paddingTop: 22, paddingBottom: 18, borderRightWidth: 1, borderRightColor: "#213958" },
  sidebarBrand: { flexDirection: "row", alignItems: "center", gap: 10, paddingHorizontal: 8, marginBottom: 22 },
  sidebarBrandMark: { width: 38, height: 38, borderRadius: 13, alignItems: "center", justifyContent: "center", backgroundColor: colors.blue },
  sidebarBrandMarkText: { color: "white", fontSize: 19, fontWeight: "900" },
  sidebarBrandName: { color: "white", fontSize: 18, fontWeight: "900", letterSpacing: -0.4 },
  sidebarBrandTagline: { color: "#8EA6C7", fontSize: 10, marginTop: 1 },
  sidebarOrganization: { backgroundColor: "#142D4F", borderRadius: 14, padding: 12, gap: 4, marginBottom: 14, borderWidth: 1, borderColor: "#244164" },
  sidebarOrganizationLabel: { color: "#7896BC", fontSize: 8, fontWeight: "800", letterSpacing: 0.9 },
  sidebarOrganizationName: { color: "white", fontSize: 13, fontWeight: "800" },
  sidebarNav: { flex: 1, gap: 4 },
  sidebarItem: { minHeight: 54, flexDirection: "row", alignItems: "center", gap: 10, paddingHorizontal: 9, borderRadius: 13 },
  sidebarItemActive: { backgroundColor: "#1A3A65" },
  sidebarItemPressed: { opacity: 0.76 },
  sidebarIcon: { width: 32, height: 32, borderRadius: 10, alignItems: "center", justifyContent: "center", backgroundColor: "#172F50" },
  sidebarIconActive: { backgroundColor: colors.blue },
  sidebarIconText: { color: "#AABDD7", fontSize: 15, fontWeight: "800" },
  sidebarIconTextActive: { color: "white" },
  sidebarItemCopy: { flex: 1, gap: 1 },
  sidebarItemLabel: { color: "#C7D4E6", fontSize: 12, fontWeight: "800" },
  sidebarItemLabelActive: { color: "white" },
  sidebarItemDescription: { color: "#7892B4", fontSize: 9 },
  sidebarPlan: { padding: 13, borderRadius: 14, backgroundColor: "#142D4F", borderWidth: 1, borderColor: "#244164", gap: 3, marginTop: 12 },
  sidebarPlanActive: { borderColor: colors.blue, backgroundColor: "#183963" },
  sidebarPlanTop: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  sidebarPlanEyebrow: { color: "#84A7D2", fontSize: 8, fontWeight: "800", letterSpacing: 0.8 },
  sidebarPlanArrow: { color: "#8EAEFF", fontSize: 15, fontWeight: "800" },
  sidebarPlanName: { color: "white", fontSize: 12, fontWeight: "800" },
  sidebarPlanCopy: { color: "#8EA6C7", fontSize: 9 },
  sidebarAccount: { flexDirection: "row", alignItems: "center", gap: 9, paddingHorizontal: 8, paddingTop: 16 },
  sidebarAvatar: { width: 34, height: 34, borderRadius: 12, alignItems: "center", justifyContent: "center", backgroundColor: "#DCE7FF" },
  sidebarAvatarText: { color: colors.navy, fontWeight: "900", fontSize: 13 },
  sidebarAccountCopy: { flex: 1, gap: 1 },
  sidebarAccountName: { color: "white", fontSize: 11, fontWeight: "800" },
  sidebarAccountRole: { color: "#7892B4", fontSize: 9 },
  center: { flex: 1, alignItems: "center", justifyContent: "center" },
  authFlow: { flex: 1, backgroundColor: colors.canvas },
  auth: { flex: 1, backgroundColor: colors.canvas },
  authDesktop: { backgroundColor: "#F5F8FC" },
  authScroll: { flexGrow: 1, padding: 20, gap: 20, justifyContent: "center" },
  authScrollDesktop: { padding: 0, gap: 0, justifyContent: "flex-start" },
  authLayout: { width: "100%", maxWidth: 520, alignSelf: "center" },
  authLayoutDesktop: { maxWidth: undefined, minHeight: "100%", flexDirection: "row", alignSelf: "stretch" },
  authVisual: { width: "54%", minHeight: 760, paddingHorizontal: 56, paddingVertical: 42, justifyContent: "space-between", backgroundColor: "#0D2341", overflow: "hidden" },
  authVisualGlow: { position: "absolute", width: 520, height: 520, borderRadius: 260, right: -180, top: -190, backgroundColor: "#173F77", opacity: 0.48 },
  authBrandDesktop: { flexDirection: "row", alignItems: "center", gap: 11, zIndex: 1 },
  brandMarkDesktop: { width: 40, height: 40, borderRadius: 13 },
  brandNameDesktop: { color: "white", fontSize: 19, fontWeight: "900", letterSpacing: -0.4 },
  brandCaptionDesktop: { color: "#8EA6C7", fontSize: 10, marginTop: 1 },
  authVisualContent: { width: "100%", maxWidth: 560, gap: 13, zIndex: 1 },
  authVisualEyebrow: { color: "#8EAEFF", fontSize: 10, fontWeight: "900", letterSpacing: 1.2 },
  authVisualTitle: { color: "white", fontSize: 42, lineHeight: 49, fontWeight: "900", letterSpacing: -1.2 },
  authVisualCopy: { color: "#C3D3E8", fontSize: 16, lineHeight: 24, maxWidth: 500 },
  authBenefits: { gap: 10, marginTop: 18 },
  authBenefit: { flexDirection: "row", alignItems: "center", gap: 12, minHeight: 66, paddingHorizontal: 13, paddingVertical: 11, borderRadius: 15, backgroundColor: "rgba(29,58,99,0.74)", borderWidth: 1, borderColor: "#29486C" },
  authBenefitIcon: { width: 38, height: 38, borderRadius: 12, alignItems: "center", justifyContent: "center", backgroundColor: colors.blue },
  authBenefitIconText: { color: "white", fontSize: 16, fontWeight: "900" },
  authBenefitCopy: { flex: 1, gap: 2 },
  authBenefitTitle: { color: "white", fontSize: 12, fontWeight: "800" },
  authBenefitText: { color: "#9EB2CE", fontSize: 10, lineHeight: 14 },
  authTrustPill: { alignSelf: "flex-start", flexDirection: "row", alignItems: "center", gap: 8, paddingHorizontal: 12, minHeight: 34, borderRadius: 12, backgroundColor: "#142D4F", borderWidth: 1, borderColor: "#244164", zIndex: 1 },
  authTrustDot: { width: 7, height: 7, borderRadius: 4, backgroundColor: colors.success },
  authTrustText: { color: "#B9C9DE", fontSize: 10, fontWeight: "700" },
  authFormColumn: { width: "100%" },
  authFormColumnDesktop: { width: "46%", minHeight: 760, alignItems: "center", justifyContent: "center", paddingHorizontal: 48, paddingVertical: 42, backgroundColor: "#F7F9FC" },
  authFormInner: { width: "100%", gap: 20 },
  authFormInnerDesktop: { maxWidth: 440, gap: 22 },
  authBrand: { flexDirection: "row", alignItems: "center", gap: 8 },
  brandMark: { width: 30, height: 30, borderRadius: 10, alignItems: "center", justifyContent: "center", backgroundColor: colors.blue },
  brandMarkText: { color: "white", fontWeight: "900", fontSize: 16 },
  brandName: { color: colors.ink, fontWeight: "800", fontSize: 17, letterSpacing: -0.3 },
  authHero: { gap: 7 },
  authEyebrow: { color: colors.blue, fontSize: 10, fontWeight: "800", letterSpacing: 1 },
  authTitle: { fontSize: 30, lineHeight: 36, fontWeight: "800", color: colors.ink, letterSpacing: -0.7 },
  authTitleDesktop: { fontSize: 34, lineHeight: 41, letterSpacing: -0.9 },
  authCopy: { color: colors.muted, fontSize: 15, lineHeight: 22 },
  authPanel: { backgroundColor: "white", borderWidth: 1, borderColor: "#E4EAF1", borderRadius: 18, padding: 16, gap: 12 },
  authPanelDesktop: { borderRadius: 20, padding: 22, gap: 14, borderColor: "#DFE6EF" },
  authSecureRow: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 2 },
  authSecureIcon: { width: 20, height: 20, borderRadius: 7, alignItems: "center", justifyContent: "center", backgroundColor: "#E5F7EF" },
  authSecureIconText: { color: colors.success, fontSize: 10, fontWeight: "900" },
  authSecureText: { flex: 1, color: colors.muted, fontSize: 10, lineHeight: 14 },
  authDivider: { flexDirection: "row", alignItems: "center", gap: 8, marginVertical: 2 },
  authDividerLine: { flex: 1, height: 1, backgroundColor: "#E8EDF3" },
  authDividerText: { color: "#98A2B3", fontSize: 8, fontWeight: "800", letterSpacing: 0.5 },
  authFooter: { color: "#98A2B3", fontSize: 10, textAlign: "center" },
  header: { paddingHorizontal: 20, paddingVertical: 16, flexDirection: "row", justifyContent: "space-between", alignItems: "center", backgroundColor: colors.navy },
  headerDesktop: { minHeight: 88, paddingHorizontal: 28, paddingVertical: 15, backgroundColor: "rgba(255,255,255,0.96)", borderBottomWidth: 1, borderBottomColor: "#E2E8F0" },
  headerEyebrow: { color: "#9CB9FF", fontSize: 11, fontWeight: "800", letterSpacing: 0.4, textTransform: "uppercase" },
  headerEyebrowDesktop: { color: colors.blue, fontSize: 9, letterSpacing: 0.8 },
  headerTitle: { color: "white", fontSize: 25, lineHeight: 31, fontWeight: "800", letterSpacing: -0.5 },
  headerTitleDesktop: { color: colors.ink, fontSize: 24, lineHeight: 30 },
  headerActions: { flexDirection: "row", alignItems: "center", gap: 10 },
  livePill: { flexDirection: "row", alignItems: "center", gap: 7, minHeight: 34, paddingHorizontal: 12, borderRadius: 12, backgroundColor: "#F5F8FC", borderWidth: 1, borderColor: "#E1E8F0" },
  liveDot: { width: 7, height: 7, borderRadius: 4, backgroundColor: colors.success },
  livePillText: { color: "#475467", fontSize: 10, fontWeight: "800" },
  headerUser: { color: "#DCE8FF", backgroundColor: "#1D3A63", borderRadius: 20, overflow: "hidden", paddingHorizontal: 11, paddingVertical: 7, fontSize: 12, fontWeight: "800" },
  headerUserDesktop: { color: colors.ink, backgroundColor: "white", borderWidth: 1, borderColor: "#DCE4ED", paddingHorizontal: 13, paddingVertical: 8 },
  dashboardScroll: { padding: 16, gap: 14, paddingBottom: 28 },
  dashboardScrollDesktop: { width: "100%", maxWidth: 1320, alignSelf: "center", paddingHorizontal: 28, paddingTop: 24, paddingBottom: 40, gap: 16 },
  dashboardHero: { backgroundColor: colors.navy, borderRadius: 20, padding: 18, gap: 8, overflow: "hidden" },
  dashboardHeroDesktop: { minHeight: 188, justifyContent: "center", paddingHorizontal: 28, paddingVertical: 24, borderRadius: 22 },
  dashboardEyebrow: { color: "#8EAEFF", fontSize: 10, fontWeight: "800", letterSpacing: 1 },
  dashboardTitle: { color: "white", fontSize: 24, lineHeight: 30, fontWeight: "800", letterSpacing: -0.5 },
  dashboardCopy: { color: "#D8E6FF", fontSize: 14, lineHeight: 20 },
  dashboardHeroActions: { flexDirection: "row", gap: 8, marginTop: 6 },
  dashboardPrimaryAction: { minHeight: 40, paddingHorizontal: 14, borderRadius: 11, alignItems: "center", justifyContent: "center", backgroundColor: colors.blue },
  dashboardPrimaryActionText: { color: "white", fontSize: 12, fontWeight: "800" },
  dashboardSecondaryAction: { minHeight: 40, paddingHorizontal: 14, borderRadius: 11, alignItems: "center", justifyContent: "center", backgroundColor: "#1D3A63" },
  dashboardSecondaryActionText: { color: "#D8E6FF", fontSize: 12, fontWeight: "800" },
  quickActionsGrid: { gap: 10 },
  quickActionsGridDesktop: { flexDirection: "row" },
  quickActionCard: { flexDirection: "row", alignItems: "center", gap: 10, minHeight: 68, padding: 12, borderRadius: 15, backgroundColor: "white", borderWidth: 1, borderColor: "#E4EAF1" },
  quickActionCardDesktop: { flex: 1, minHeight: 78, paddingHorizontal: 16 },
  quickActionIcon: { width: 34, height: 34, borderRadius: 11, alignItems: "center", justifyContent: "center", backgroundColor: colors.blueSoft },
  quickActionIconDark: { backgroundColor: "#EAF0F7" },
  quickActionIconText: { color: colors.blue, fontSize: 16, fontWeight: "900" },
  quickActionCopy: { flex: 1, gap: 2 },
  quickActionTitle: { color: colors.ink, fontSize: 13, fontWeight: "800" },
  quickActionText: { color: colors.muted, fontSize: 10, lineHeight: 14 },
  quickActionArrow: { color: colors.blue, fontSize: 18, fontWeight: "700" },
  kpiGrid: { flexDirection: "row", flexWrap: "wrap", gap: 10 },
  kpiCard: { width: "48.5%", minHeight: 126, backgroundColor: "white", borderRadius: 16, borderWidth: 1, borderColor: "#E4EAF1", padding: 14, gap: 5 },
  kpiCardDesktop: { width: undefined, flex: 1, minWidth: 180, minHeight: 132, padding: 17 },
  kpiDot: { width: 9, height: 9, borderRadius: 5, backgroundColor: "#98A2B3", marginBottom: 2 },
  kpiDotBlue: { backgroundColor: colors.blue },
  kpiDotGreen: { backgroundColor: colors.success },
  kpiDotOrange: { backgroundColor: "#F79009" },
  kpiLabel: { color: colors.muted, fontSize: 11, fontWeight: "700" },
  kpiValue: { color: colors.ink, fontSize: 21, lineHeight: 27, fontWeight: "800", letterSpacing: -0.5 },
  kpiDetail: { color: colors.muted, fontSize: 10, lineHeight: 14 },
  insightPanel: { backgroundColor: "white", borderRadius: 16, borderWidth: 1, borderColor: "#E4EAF1", padding: 14, gap: 13 },
  insightGrid: { gap: 14 },
  insightGridDesktop: { flexDirection: "row", alignItems: "stretch", gap: 16 },
  insightPanelDesktop: { flex: 1, minHeight: 220, padding: 18 },
  sectionHeaderRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: 10 },
  sectionEyebrow: { color: colors.blue, fontSize: 9, fontWeight: "800", letterSpacing: 0.9, marginBottom: 3 },
  sectionLink: { color: colors.blue, fontSize: 11, fontWeight: "800" },
  funnelList: { gap: 11 },
  funnelRow: { flexDirection: "row", alignItems: "center", gap: 8 },
  funnelLabel: { width: 72, color: "#475467", fontSize: 11, fontWeight: "700" },
  funnelTrack: { flex: 1, height: 7, borderRadius: 7, backgroundColor: "#EDF1F6", overflow: "hidden" },
  funnelFill: { height: 7, borderRadius: 7, backgroundColor: colors.blue },
  funnelCount: { width: 20, textAlign: "right", color: colors.ink, fontSize: 11, fontWeight: "800" },
  deadlineInsightRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingTop: 10, borderTopWidth: 1, borderTopColor: "#EEF2F6" },
  deadlineInsightDate: { width: 58, minHeight: 40, alignItems: "center", justifyContent: "center", borderRadius: 10, backgroundColor: "#FFF4E5", paddingHorizontal: 5 },
  deadlineInsightDateText: { color: colors.warning, fontSize: 9, lineHeight: 13, fontWeight: "800", textAlign: "center" },
  deadlineInsightCopy: { flex: 1, gap: 2 },
  deadlineInsightTitle: { color: colors.ink, fontSize: 12, lineHeight: 17, fontWeight: "700" },
  usageStrip: { backgroundColor: colors.blueSoft, borderRadius: 14, padding: 14, gap: 7 },
  usageStripTitle: { color: colors.ink, fontSize: 12, fontWeight: "800" },
  usageStripValue: { position: "absolute", right: 14, top: 14, color: colors.blue, fontSize: 12, fontWeight: "800" },
  usageStripTrack: { height: 7, borderRadius: 7, overflow: "hidden", backgroundColor: "#CFDCFA" },
  usageStripFill: { height: 7, borderRadius: 7, backgroundColor: colors.blue },
  toolbar: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: 16, paddingVertical: 14 },
  toolbarDesktop: { width: "100%", maxWidth: 1440, alignSelf: "center", paddingHorizontal: 28, paddingVertical: 18 },
  segmentedControl: { flexDirection: "row", padding: 3, gap: 2, backgroundColor: "#E9EEF5", borderRadius: 12 },
  segment: { minWidth: 72, alignItems: "center", paddingHorizontal: 12, paddingVertical: 8, borderRadius: 9 },
  segmentActive: { backgroundColor: "white", borderWidth: 1, borderColor: "#E4EAF1" },
  segmentText: { color: colors.muted, fontSize: 13, fontWeight: "700" },
  segmentTextActive: { color: colors.ink },
  addButton: { backgroundColor: colors.blue, minHeight: 38, paddingHorizontal: 14, borderRadius: 12, alignItems: "center", justifyContent: "center" },
  addButtonText: { color: "white", fontSize: 13, fontWeight: "800" },
  kanban: { paddingHorizontal: 16, gap: 12, paddingBottom: 18 },
  kanbanDesktop: { paddingHorizontal: 28, gap: 14, paddingBottom: 30 },
  column: { width: 288, backgroundColor: "#EAF0F7", borderRadius: 18, padding: 10, gap: 10 },
  columnDesktop: { width: 310, borderRadius: 20, padding: 12 },
  columnHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: 4, paddingTop: 2 },
  columnTitle: { fontWeight: "800", fontSize: 15, color: colors.ink, letterSpacing: -0.2 },
  columnCount: { minWidth: 22, textAlign: "center", color: "#475467", fontSize: 11, fontWeight: "800", backgroundColor: "#D8E1EC", paddingVertical: 3, paddingHorizontal: 6, borderRadius: 12, overflow: "hidden" },
  draggableCard: { gap: 5 },
  draggableCardActive: { opacity: 0.88, zIndex: 5 },
  dragHandle: { height: 20, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 3 },
  dragHandleDot: { width: 3, height: 3, borderRadius: 2, backgroundColor: "#98A2B3" },
  dragHint: { color: "#98A2B3", fontSize: 9, fontWeight: "700", marginLeft: 3 },
  columnEmpty: { minHeight: 84, alignItems: "center", justifyContent: "center", gap: 4, borderRadius: 13, borderWidth: 1, borderStyle: "dashed", borderColor: "#C8D2DE" },
  columnEmptyIcon: { color: "#98A2B3", fontSize: 18, fontWeight: "500" },
  columnEmptyText: { color: "#98A2B3", fontSize: 10, fontWeight: "700" },
  listEmpty: { alignItems: "center", justifyContent: "center", padding: 28, gap: 6 },
  pipelineList: { padding: 16, gap: 10 },
  pipelineListDesktop: { width: "100%", maxWidth: 980, alignSelf: "center", paddingHorizontal: 28, paddingBottom: 36 },
  dealCard: { backgroundColor: "white", borderRadius: 14, padding: 14, gap: 9, borderWidth: 1, borderColor: "#E4EAF1" },
  dealCardHeader: { flexDirection: "row", alignItems: "flex-start", justifyContent: "space-between", gap: 10 },
  dealTitle: { flex: 1, fontWeight: "700", fontSize: 15, lineHeight: 21, color: colors.ink, letterSpacing: -0.2 },
  stageBadge: { color: "#475467", backgroundColor: "#F2F5F8", borderRadius: 7, overflow: "hidden", paddingHorizontal: 7, paddingVertical: 4, fontSize: 10, fontWeight: "700" },
  dealMeta: { color: colors.muted, fontSize: 12 },
  lastFollowUp: { color: "#475467", fontSize: 12, lineHeight: 18, fontStyle: "italic" },
  input: { backgroundColor: "white", borderWidth: 1, borderColor: "#D8E1EC", color: colors.ink, borderRadius: 12, paddingHorizontal: 13, minHeight: 48, fontSize: 15 },
  fieldLabel: { fontWeight: "700", color: colors.ink, fontSize: 13 },
  primaryBtn: { backgroundColor: colors.blue, borderRadius: 12, minHeight: 46, paddingHorizontal: 16, alignItems: "center", justifyContent: "center" },
  primaryText: { color: "white", fontWeight: "800" },
  secondaryBtn: { borderWidth: 1, borderColor: colors.border, borderRadius: 12, minHeight: 44, alignItems: "center", justifyContent: "center", backgroundColor: "white" },
  secondaryText: { color: colors.ink, fontWeight: "700" },
  buttonPressed: { opacity: 0.76, transform: [{ scale: 0.992 }] },
  dangerBtn: { borderWidth: 1, borderColor: "#F4C7BC", borderRadius: 12, minHeight: 40, alignItems: "center", justifyContent: "center", backgroundColor: "#FFF5F2", paddingHorizontal: 10 },
  dangerText: { color: colors.danger, fontWeight: "700", fontSize: 12 },
  dealActions: { flexDirection: "row", alignItems: "center", gap: 8, paddingTop: 2 },
  cardUtilityRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingTop: 2 },
  followUpComposer: { gap: 8, paddingTop: 3 },
  followUpActions: { gap: 4 },
  followUpCancel: { minHeight: 32, alignItems: "center", justifyContent: "center" },
  followUpCancelText: { color: colors.muted, fontSize: 11, fontWeight: "700" },
  cardLink: { alignSelf: "flex-start", paddingVertical: 3 },
  cardLinkText: { color: colors.blue, fontSize: 12, fontWeight: "800" },
  linkText: { color: colors.blue, fontWeight: "700", fontSize: 12 },
  historyBox: { gap: 8, paddingTop: 4 },
  historyItem: { backgroundColor: colors.canvas, borderRadius: 8, padding: 8, gap: 2 },
  historyDate: { color: colors.muted, fontSize: 10, fontWeight: "700" },
  historyNote: { color: colors.ink, fontSize: 12, lineHeight: 17 },
  linkedBadge: { color: colors.blue, fontSize: 11, fontWeight: "800" },
  settingsPanel: { backgroundColor: "white", borderRadius: 14, padding: 16, gap: 8, borderWidth: 1, borderColor: colors.border },
  settingRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingVertical: 12, gap: 12 },
  settingBorder: { borderTopWidth: 1, borderTopColor: colors.border },
  settingCopy: { flex: 1 },
  inPipeline: { color: colors.success, fontWeight: "800", fontSize: 12 },
  pageScroll: { padding: 16, gap: 14, paddingBottom: 28 },
  pageScrollDesktop: { width: "100%", maxWidth: 1180, alignSelf: "center", paddingHorizontal: 28, paddingTop: 24, paddingBottom: 40, gap: 16 },
  pageScrollNarrowDesktop: { width: "100%", maxWidth: 860, alignSelf: "center", paddingHorizontal: 28, paddingTop: 24, paddingBottom: 40, gap: 16 },
  desktopPanelGrid: { gap: 14 },
  desktopPanelGridActive: { flexDirection: "row", alignItems: "flex-start", gap: 16 },
  desktopPanel: { flex: 1, minHeight: 260, padding: 18 },
  screenIntro: { gap: 5, paddingHorizontal: 2, paddingBottom: 2 },
  screenEyebrow: { color: colors.blue, fontSize: 10, fontWeight: "800", letterSpacing: 1 },
  screenTitle: { color: colors.ink, fontSize: 22, lineHeight: 28, fontWeight: "800", letterSpacing: -0.4 },
  screenCopy: { color: colors.muted, fontSize: 14, lineHeight: 20 },
  complianceNotice: { backgroundColor: "#FFF9ED", borderRadius: 14, borderWidth: 1, borderColor: "#F3D79B", padding: 12, gap: 3 },
  complianceNoticeCompact: { marginTop: 6, marginBottom: 2 },
  complianceNoticeTitle: { color: "#8A5A00", fontSize: 11, fontWeight: "800" },
  complianceNoticeText: { color: "#735F37", fontSize: 11, lineHeight: 16 },
  legalFooter: { marginTop: 20, gap: 6, alignItems: "center" },
  legalFooterLinks: { flexDirection: "row", alignItems: "center", gap: 8, flexWrap: "wrap", justifyContent: "center" },
  legalFooterMuted: { color: "#98A2B3", fontSize: 11 },
  legalLink: { color: colors.blue, fontWeight: "700", fontSize: 12 },
  legalAcceptRow: { flexDirection: "row", alignItems: "flex-start", gap: 10, marginTop: 4, marginBottom: 8 },
  legalAcceptText: { flex: 1, color: "#475467", fontSize: 12, lineHeight: 18 },
  radarBadges: { flexDirection: "row", alignItems: "center", gap: 6 },
  demoBadge: { backgroundColor: "#FEF0C7", color: "#93370D", fontSize: 10, fontWeight: "800", paddingHorizontal: 8, paddingVertical: 3, borderRadius: 6, overflow: "hidden" },
  formPanel: { backgroundColor: "white", borderRadius: 16, borderWidth: 1, borderColor: "#E4EAF1", padding: 14, gap: 10 },
  formSectionLabel: { color: colors.blue, fontSize: 9, fontWeight: "800", letterSpacing: 0.9, marginBottom: 2 },
  fieldHint: { color: colors.muted, fontSize: 11, marginTop: -7 },
  valueFieldsRow: { flexDirection: "row", gap: 10 },
  valueField: { flex: 1 },
  advancedToggle: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: 12, padding: 14, borderRadius: 16, backgroundColor: "white", borderWidth: 1, borderColor: "#E4EAF1" },
  advancedToggleTitle: { color: colors.ink, fontSize: 13, fontWeight: "800" },
  advancedToggleCopy: { color: colors.muted, fontSize: 10, marginTop: 2 },
  advancedToggleIcon: { width: 28, height: 28, textAlign: "center", textAlignVertical: "center", borderRadius: 9, overflow: "hidden", backgroundColor: colors.blueSoft, color: colors.blue, fontSize: 18, lineHeight: 27, fontWeight: "700" },
  profilePreview: { flexDirection: "row", alignItems: "flex-start", gap: 11, padding: 14, borderRadius: 16, backgroundColor: colors.navy },
  profilePreviewIcon: { width: 36, height: 36, alignItems: "center", justifyContent: "center", borderRadius: 12, backgroundColor: colors.blue },
  profilePreviewIconText: { color: "white", fontSize: 15, fontWeight: "900" },
  profilePreviewCopy: { flex: 1, gap: 3 },
  profilePreviewTitle: { color: "white", fontSize: 13, fontWeight: "800" },
  profilePreviewText: { color: "#D8E6FF", fontSize: 11, lineHeight: 16 },
  radarList: { padding: 16, gap: 10, paddingBottom: 28 },
  radarListDesktop: { width: "100%", maxWidth: 1240, alignSelf: "center", paddingHorizontal: 28, paddingTop: 24, paddingBottom: 40 },
  radarGridRow: { gap: 14 },
  radarTop: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  radarCard: { backgroundColor: "white", borderRadius: 16, padding: 14, gap: 10, borderWidth: 1, borderColor: "#E4EAF1" },
  radarCardDesktop: { flex: 1, minHeight: 190, padding: 18, borderRadius: 18 },
  radarTitle: { color: colors.ink, fontWeight: "700", fontSize: 16, lineHeight: 22, letterSpacing: -0.2, marginTop: 9 },
  radarValue: { color: colors.ink, fontSize: 14, fontWeight: "800", marginTop: 2 },
  radarAction: { alignSelf: "flex-start", minHeight: 34, justifyContent: "center", paddingHorizontal: 2 },
  radarActionText: { color: colors.blue, fontSize: 13, fontWeight: "800" },
  alertCard: { flexDirection: "row", alignItems: "center", gap: 10, backgroundColor: colors.blueSoft, borderRadius: 16, padding: 12, borderWidth: 1, borderColor: "#D5E1FF", marginTop: 4 },
  alertIcon: { width: 36, height: 36, borderRadius: 12, alignItems: "center", justifyContent: "center", backgroundColor: colors.blue },
  alertIconLarge: { width: 42, height: 42, borderRadius: 14, alignItems: "center", justifyContent: "center", backgroundColor: colors.blueSoft },
  alertIconText: { color: "white", fontSize: 16, fontWeight: "900" },
  alertIconTextLarge: { color: colors.blue },
  alertCardCopy: { flex: 1, gap: 2 },
  alertCardTitle: { color: colors.ink, fontSize: 13, fontWeight: "800" },
  alertCardText: { color: colors.muted, fontSize: 11, lineHeight: 15 },
  alertConfigureButton: { minHeight: 34, paddingHorizontal: 11, borderRadius: 10, alignItems: "center", justifyContent: "center", backgroundColor: "white" },
  alertConfigureText: { color: colors.blue, fontSize: 11, fontWeight: "800" },
  alertSheet: { backgroundColor: "white", borderTopLeftRadius: 24, borderTopRightRadius: 24, paddingHorizontal: 20, paddingTop: 10, paddingBottom: 24, gap: 12 },
  alertSheetHeader: { flexDirection: "row", alignItems: "center", gap: 11 },
  alertSheetHeaderCopy: { flex: 1, gap: 2 },
  alertChannelRow: { flexDirection: "row", alignItems: "center", gap: 12, backgroundColor: colors.canvas, borderRadius: 14, padding: 12 },
  alertChannelCopy: { flex: 1, gap: 2 },
  alertChannelTitle: { color: colors.ink, fontSize: 13, fontWeight: "800" },
  alertChannelText: { color: colors.muted, fontSize: 10, lineHeight: 14 },
  alertStatusHero: { flexDirection: "row", alignItems: "center", gap: 12, padding: 16, borderRadius: 18, backgroundColor: colors.navy },
  alertStatusOrb: { width: 46, height: 46, borderRadius: 16, alignItems: "center", justifyContent: "center", backgroundColor: colors.blue },
  alertStatusOrbText: { color: "white", fontSize: 20, fontWeight: "900" },
  alertStatusCopy: { flex: 1, gap: 3 },
  alertStatusLabel: { color: "#8EAEFF", fontSize: 9, fontWeight: "800", letterSpacing: 0.8 },
  alertStatusTitle: { color: "white", fontSize: 15, lineHeight: 20, fontWeight: "800" },
  alertStatusText: { color: "#D8E6FF", fontSize: 11, lineHeight: 15 },
  alertStatsRow: { flexDirection: "row", gap: 10 },
  alertStat: { flex: 1, padding: 12, borderRadius: 14, backgroundColor: "white", borderWidth: 1, borderColor: "#E4EAF1", gap: 3 },
  alertStatValue: { color: colors.ink, fontSize: 20, fontWeight: "800" },
  alertStatLabel: { color: colors.muted, fontSize: 10, lineHeight: 13 },
  termCloud: { flexDirection: "row", flexWrap: "wrap", gap: 7 },
  termChip: { backgroundColor: colors.blueSoft, borderRadius: 9, paddingHorizontal: 9, paddingVertical: 6 },
  termChipText: { color: colors.blue, fontSize: 11, fontWeight: "800" },
  alertFilterLine: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", gap: 10, paddingTop: 10, borderTopWidth: 1, borderTopColor: "#EEF2F6" },
  alertFilterLabel: { color: colors.muted, fontSize: 11, fontWeight: "700" },
  alertFilterValue: { color: colors.ink, fontSize: 12, fontWeight: "800", textAlign: "right", flex: 1 },
  alertChannelStatus: { flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 9, borderTopWidth: 1, borderTopColor: "#EEF2F6" },
  alertChannelDot: { width: 9, height: 9, borderRadius: 5, backgroundColor: "#C5CED9" },
  alertChannelDotEnabled: { backgroundColor: colors.success },
  alertChannelStatusCopy: { flex: 1, gap: 1 },
  alertChannelState: { color: colors.muted, fontSize: 10, fontWeight: "800" },
  alertChannelStateEnabled: { color: colors.success },
  activitySummary: { flexDirection: "row", alignItems: "center", gap: 12, padding: 15, borderRadius: 16, backgroundColor: colors.blueSoft, borderWidth: 1, borderColor: "#D5E1FF" },
  activitySummaryValue: { color: colors.blue, fontSize: 30, lineHeight: 34, fontWeight: "900" },
  activitySummaryTitle: { color: colors.ink, fontSize: 13, fontWeight: "800" },
  activityTimeline: { backgroundColor: "white", borderRadius: 16, padding: 14, borderWidth: 1, borderColor: "#E4EAF1" },
  activityItem: { flexDirection: "row", gap: 11, minHeight: 78 },
  activityRail: { width: 30, alignItems: "center", position: "relative" },
  activityRailLine: { position: "absolute", top: 30, bottom: -8, width: 1, backgroundColor: "#DCE4ED" },
  activityIcon: { width: 28, height: 28, borderRadius: 10, alignItems: "center", justifyContent: "center", backgroundColor: colors.blueSoft, zIndex: 1 },
  activityIconGreen: { backgroundColor: "#E5F7EF" },
  activityIconOrange: { backgroundColor: "#FFF4E5" },
  activityIconText: { color: colors.blue, fontSize: 13, fontWeight: "900" },
  activityCopy: { flex: 1, gap: 3, paddingBottom: 14 },
  activityTitle: { color: colors.ink, fontSize: 13, fontWeight: "800" },
  activityDetail: { color: colors.muted, fontSize: 11, lineHeight: 15 },
  activityDate: { color: "#98A2B3", fontSize: 10, fontWeight: "700" },
  badge: { color: colors.blue, fontWeight: "800", fontSize: 11, backgroundColor: colors.blueSoft, paddingHorizontal: 8, paddingVertical: 4, borderRadius: 8, overflow: "hidden" },
  deadline: { color: colors.muted, fontSize: 11, fontWeight: "700" },
  deadlineUrgent: { color: colors.warning },
  modalSafe: { flex: 1, backgroundColor: colors.canvas },
  sourceCard: { borderTopWidth: 1, borderTopColor: colors.border, marginTop: 22, paddingTop: 16 },
  metaLabel: { color: colors.muted, fontSize: 9, fontWeight: "800", letterSpacing: 0.8 },
  modalBar: { paddingHorizontal: 20, paddingVertical: 14, flexDirection: "row", justifyContent: "space-between", alignItems: "center", borderBottomWidth: 1, borderBottomColor: colors.border, backgroundColor: "white" },
  modalEyebrow: { color: colors.blue, fontSize: 10, letterSpacing: 0.8, fontWeight: "800" },
  modalBarTitle: { color: colors.ink, fontSize: 14, fontWeight: "800", marginTop: 2 },
  modalClose: { backgroundColor: colors.blueSoft, borderRadius: 10, paddingHorizontal: 11, paddingVertical: 8 },
  modalBack: { color: colors.blue, fontWeight: "800", fontSize: 12 },
  modalContent: { padding: 20, gap: 14, paddingBottom: 30 },
  modalTitle: { fontSize: 22, fontWeight: "800", color: colors.ink, lineHeight: 28 },
  summaryRow: { flexDirection: "row", gap: 10 },
  summaryChip: { flex: 1, backgroundColor: "white", borderRadius: 12, padding: 12, borderWidth: 1, borderColor: colors.border },
  callout: { backgroundColor: colors.navy, borderRadius: 16, padding: 16, gap: 10 },
  calloutTitle: { color: "white", fontWeight: "800", fontSize: 18 },
  calloutCopy: { color: "#D8E6FF", fontSize: 14, lineHeight: 20 },
  analysisBox: { backgroundColor: "white", borderRadius: 12, padding: 14, borderWidth: 1, borderColor: colors.border, gap: 6 },
  analysisScore: { color: colors.success, fontWeight: "800", fontSize: 18 },
  analysisDisclaimer: { color: colors.muted, fontSize: 10, lineHeight: 14, marginTop: 3, marginBottom: 8 },
  sourceMeta: { color: colors.muted, fontSize: 11, marginTop: 6 },
  sourceWarning: { color: "#9A5B00", fontSize: 11, lineHeight: 15, marginTop: 5 },
  planHero: { backgroundColor: colors.navy, borderRadius: 16, padding: 18, gap: 6 },
  planEyebrow: { color: "#8DB1FF", fontSize: 10, fontWeight: "800" },
  planName: { color: "white", fontSize: 28, fontWeight: "900" },
  planOrganization: { color: "#D8E6FF", fontSize: 14 },
  usageGrid: { flexDirection: "row", gap: 8 },
  usageMetric: { flex: 1, minHeight: 82, justifyContent: "center", gap: 4, padding: 10, backgroundColor: "white", borderRadius: 14, borderWidth: 1, borderColor: "#E4EAF1" },
  usageMetricValue: { color: colors.ink, fontSize: 18, fontWeight: "800", letterSpacing: -0.3 },
  usageMetricLabel: { color: colors.muted, fontSize: 10, lineHeight: 14, fontWeight: "700" },
  sectionHeader: { gap: 4, paddingTop: 4 },
  sectionTitle: { color: colors.ink, fontSize: 16, fontWeight: "800", letterSpacing: -0.2 },
  sectionCopy: { color: colors.muted, fontSize: 13, lineHeight: 19 },
  textAction: { minHeight: 36, alignItems: "center", justifyContent: "center" },
  textActionLabel: { color: colors.blue, fontSize: 13, fontWeight: "800" },
  successText: { color: colors.success, fontWeight: "700" },
  errorInline: { color: colors.danger },
  accountPanel: { flexDirection: "row", alignItems: "center", gap: 12, backgroundColor: "white", borderRadius: 16, padding: 16, borderWidth: 1, borderColor: "#E4EAF1" },
  accountAvatar: { width: 44, height: 44, alignItems: "center", justifyContent: "center", borderRadius: 15, backgroundColor: colors.navy },
  accountAvatarText: { color: "white", fontSize: 17, fontWeight: "800" },
  accountCopy: { flex: 1, gap: 2 },
  accountName: { color: colors.ink, fontSize: 17, fontWeight: "800" },
  accountEmail: { color: colors.muted, fontSize: 12 },
  planShortcut: { minHeight: 70, flexDirection: "row", alignItems: "center", gap: 12, padding: 14, borderRadius: 16, backgroundColor: colors.blueSoft, borderWidth: 1, borderColor: "#D5E1FF" },
  planShortcutCopy: { flex: 1, gap: 2 },
  planShortcutArrow: { color: colors.blue, fontSize: 20, fontWeight: "700" },
  navScroll: { flexGrow: 0, maxHeight: 76, backgroundColor: "white", borderTopWidth: 1, borderTopColor: colors.border },
  nav: { flexGrow: 1, flexDirection: "row", gap: 2, paddingHorizontal: 8, paddingVertical: 7, backgroundColor: "white" },
  navItem: { width: 51, minHeight: 60, alignItems: "center", justifyContent: "center", gap: 2, paddingVertical: 7, borderRadius: 11 },
  navItemActive: { backgroundColor: colors.blueSoft },
  navSpotlight: { backgroundColor: colors.blueSoft, borderRadius: 10, marginHorizontal: 2, borderWidth: 1, borderColor: colors.blue },
  navIcon: { fontSize: 17, lineHeight: 19, color: colors.muted, fontWeight: "700" },
  navLabel: { fontSize: 9, fontWeight: "700", color: colors.muted },
  navActive: { color: colors.blue },
  navSpotlightText: { color: colors.blue },
  signOutAction: { minHeight: 44, alignItems: "center", justifyContent: "center" },
  signOutText: { color: colors.danger, fontSize: 13, fontWeight: "800" },
  muted: { color: colors.muted, lineHeight: 20 },
  error: { color: colors.danger, paddingHorizontal: 16 },
  composerOverlay: { flex: 1, justifyContent: "flex-end" },
  composerBackdrop: { ...StyleSheet.absoluteFill, backgroundColor: "rgba(16, 35, 63, 0.42)" },
  composerSheet: { backgroundColor: "white", borderTopLeftRadius: 24, borderTopRightRadius: 24, paddingHorizontal: 20, paddingTop: 10, paddingBottom: 28, gap: 14 },
  composerHandle: { alignSelf: "center", width: 38, height: 4, borderRadius: 4, backgroundColor: "#D8E1EC", marginBottom: 2 },
  composerEyebrow: { color: colors.blue, fontSize: 11, fontWeight: "800", letterSpacing: 0.8 },
  composerTitle: { color: colors.ink, fontSize: 23, lineHeight: 29, fontWeight: "800", letterSpacing: -0.4 },
  composerCopy: { color: colors.muted, fontSize: 14, lineHeight: 20, marginTop: -8 },
  composerCancel: { minHeight: 38, justifyContent: "center", alignItems: "center" },
  composerCancelText: { color: colors.muted, fontSize: 14, fontWeight: "700" },
});
