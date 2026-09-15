import { useCallback, useEffect, useMemo, useState } from "react";
import { ActivityIndicator, FlatList, Linking, Modal, Platform, Pressable, RefreshControl, SafeAreaView, ScrollView, StatusBar, StyleSheet, Switch, Text, TextInput, useWindowDimensions, View } from "react-native";
import { StatusBar as ExpoStatusBar } from "expo-status-bar";
import { Analysis, LicitaLensAPI, LicitaLensClient, Opportunity, Profile, ProfileInput, SubscriptionHistoryEntry, Usage } from "./src/api/client";
import { DemoLicitaLensClient } from "./src/api/demo";
import { authConfigured, signInWithKeycloak } from "./src/auth/session";
import { colors } from "./src/theme";
import { clearSession, defaultPreferences, loadLocalState, Preferences, saveOnboarded, savePreferences, saveSession } from "./src/storage";
import SaasFlow from "./src/app/SaasFlow";

const apiUrl = process.env.EXPO_PUBLIC_API_URL ?? "http://localhost:8080";
const isDemo = process.env.EXPO_PUBLIC_DEMO_MODE === "true";
const defaultOrganizationId = process.env.EXPO_PUBLIC_ORGANIZATION_ID ?? "00000000-0000-0000-0000-000000000001";
type Tab = "feed" | "profile" | "usage" | "settings";
type Draft = { name: string; description: string; keywords: string; states: string; minimum: string; maximum: string };
const initialDraft: Draft = { name: "", description: "", keywords: "", states: "", minimum: "", maximum: "" };
const demoUsage: Usage = { plan: "pro", profiles: { used: 0, limit: 5 }, ai_analyses: { used: 0, limit: 200 }, daily_alerts: { used: 0, limit: 100 } };

export default function App() {
  if (!isDemo) return <SaasFlow />;
  return <DemoApp />;
}

function DemoApp() {
  const [accessToken, setAccessToken] = useState<string | undefined>(() => process.env.EXPO_PUBLIC_ACCESS_TOKEN);
  const [organizationId, setOrganizationId] = useState(defaultOrganizationId);
  const [organizationName, setOrganizationName] = useState("");
  const [sessionReady, setSessionReady] = useState(isDemo);
  const client = useMemo<LicitaLensAPI>(() => isDemo ? new DemoLicitaLensClient() : new LicitaLensClient({ baseUrl: apiUrl, organizationId, accessToken }), [accessToken, organizationId]);
  const desktop = useWindowDimensions().width >= 900;
  const [tab, setTab] = useState<Tab>("feed");
  const [opportunities, setOpportunities] = useState<Opportunity[]>([]);
  const [profile, setProfile] = useState<Profile>();
  const [usage, setUsage] = useState<Usage>(demoUsage);
  const [billingHistory, setBillingHistory] = useState<SubscriptionHistoryEntry[]>([]);
  const [preferences, setPreferences] = useState<Preferences>(defaultPreferences);
  const [onboarded, setOnboarded] = useState(false);
  const [selected, setSelected] = useState<Opportunity>();
  const [analysis, setAnalysis] = useState<Analysis>();
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => {
    if (isDemo) return;
    void loadLocalState().then((local) => {
      if (local.accessToken) setAccessToken(local.accessToken);
      if (local.organizationId) setOrganizationId(local.organizationId);
      if (local.organizationName) setOrganizationName(local.organizationName);
      setSessionReady(true);
    });
  }, []);

  useEffect(() => {
    if (isDemo || !accessToken || organizationName) return;
    void (async () => {
      try {
        const probe = new LicitaLensClient({ baseUrl: apiUrl, organizationId: defaultOrganizationId, accessToken });
        const organizations = await probe.listOrganizations();
        if (!organizations.data[0]) return;
        const active = organizations.data[0];
        setOrganizationId(active.id);
        setOrganizationName(active.name);
        await saveSession(accessToken, active.id, active.name);
      } catch {
        /* login or org setup will handle */
      }
    })();
  }, [accessToken, organizationName]);

  const load = useCallback(async (refresh = false) => {
    refresh ? setRefreshing(true) : setLoading(true);
    setError(undefined);
    try {
      const local = await loadLocalState();
      const profiles = await client.listProfiles();
      const [feed, currentUsage, history] = await Promise.all([
        client.listOpportunities(profiles.data[0]?.id),
        client.getUsage().catch(() => demoUsage),
        client.billingHistory().catch(() => ({ data: [] as SubscriptionHistoryEntry[] })),
      ]);
      setPreferences(local.preferences);
      setOpportunities(feed.data);
      setProfile(profiles.data[0]);
      setUsage(currentUsage);
      setBillingHistory(history.data);
      setOnboarded(local.onboarded || Boolean(profiles.data[0]));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Não foi possível acessar o LicitaLens.");
    } finally { setLoading(false); setRefreshing(false); }
  }, [client]);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    if (!isDemo && (!accessToken || !organizationName)) setLoading(false);
  }, [accessToken, organizationName]);

  async function finishOnboarding(input: ProfileInput) {
    const created = await client.createProfile(input);
    setProfile(created);
    setUsage((current) => ({ ...current, profiles: { ...current.profiles, used: 1 } }));
    setOnboarded(true);
    await saveOnboarded(true);
  }
  async function persistProfile(input: ProfileInput) {
    const saved = profile ? await client.updateProfile(profile.id, input) : await client.createProfile(input);
    setProfile(saved);
    return saved;
  }
  async function persistPreferences(next: Preferences) { setPreferences(next); await savePreferences(next); }

  async function completeLogin(token: string) {
    setAccessToken(token);
    const probe = new LicitaLensClient({ baseUrl: apiUrl, organizationId: defaultOrganizationId, accessToken: token });
    const organizations = await probe.listOrganizations();
    if (organizations.data[0]) {
      const active = organizations.data[0];
      setOrganizationId(active.id);
      setOrganizationName(active.name);
      await saveSession(token, active.id, active.name);
      return;
    }
    setOrganizationId("");
    setOrganizationName("");
    await saveSession(token, "", "");
  }

  async function completeOrganizationSetup(name: string) {
    if (!accessToken) throw new Error("Sessão inválida.");
    const probe = new LicitaLensClient({ baseUrl: apiUrl, organizationId: defaultOrganizationId, accessToken });
    const created = await probe.bootstrapOrganization(name);
    setOrganizationId(created.id);
    setOrganizationName(created.name);
    await saveSession(accessToken, created.id, created.name);
  }

  async function openBillingUrl(action: () => Promise<{ url: string }>) {
    const { url } = await action();
    await Linking.openURL(url);
  }

  if (!isDemo && !sessionReady) return <LoadingScreen />;
  if (!isDemo && !accessToken) {
    return <LoginScreen configured={authConfigured()} onSignIn={async () => {
      const session = await signInWithKeycloak();
      await completeLogin(session.accessToken);
    }} onDevToken={async (token) => { await completeLogin(token); }} />;
  }
  if (!isDemo && accessToken && !organizationName) {
    return <OrganizationSetupScreen onComplete={completeOrganizationSetup} />;
  }

  if (loading) return <LoadingScreen />;
  if (error) return <ConnectionScreen message={error} onRetry={() => void load()} />;
  if (!onboarded || !profile) return <Onboarding onComplete={finishOnboarding} />;

  const content = tab === "feed"
    ? <Feed items={opportunities} refreshing={refreshing} onRefresh={() => void load(true)} onSelect={(item) => { setSelected(item); setAnalysis(undefined); }} />
    : tab === "profile" ? <ProfileScreen profile={profile} onSave={persistProfile} />
    : tab === "usage" ? <UsageScreen usage={usage} history={billingHistory} organizationName={organizationName || "Sua empresa"} onUpgradeEssential={() => openBillingUrl(() => client.createCheckout("essential"))} onUpgradePro={() => openBillingUrl(() => client.createCheckout("pro"))} onManage={() => openBillingUrl(() => client.createPortal())} />
    : <SettingsScreen preferences={preferences} onChange={persistPreferences} onSignOut={isDemo ? undefined : async () => { await clearSession(); setAccessToken(undefined); setOrganizationName(""); setOnboarded(false); }} adminUrl={`${apiUrl}/admin`} />;

  return <SafeAreaView style={styles.app}>
    <ExpoStatusBar style={desktop ? "dark" : "light"} />
    <View style={[styles.shell, desktop && styles.shellDesktop]}>
      {desktop && <Navigation active={tab} onChange={setTab} desktop />}
      <View style={styles.main}>{content}</View>
      {!desktop && <Navigation active={tab} onChange={setTab} />}
    </View>
    <OpportunityDetail item={selected} analysis={analysis} onClose={() => setSelected(undefined)} onAnalyze={async () => {
      if (!selected || !profile) return;
      const result = await client.analyze(selected.id, profile.id);
      setAnalysis(result);
      if (result.matched) setUsage((current) => ({ ...current, ai_analyses: { ...current.ai_analyses, used: current.ai_analyses.used + 1 } }));
    }} />
  </SafeAreaView>;
}

function LoadingScreen() { return <SafeAreaView style={styles.centered}><ExpoStatusBar style="dark" /><View style={styles.brandMark}><Text style={styles.brandMarkText}>L</Text></View><Text style={styles.brandName}>LicitaLens</Text><ActivityIndicator style={{ marginTop: 28 }} color={colors.blue} /></SafeAreaView>; }
function ConnectionScreen({ message, onRetry }: { message: string; onRetry: () => void }) { return <SafeAreaView style={styles.centered}><View style={styles.noticeIcon}><Text style={styles.noticeIconText}>!</Text></View><Text style={styles.emptyTitle}>Não conseguimos conectar</Text><Text style={styles.emptyText}>{message}</Text><Text style={styles.helper}>Confirme que o backend local está rodando em {apiUrl}.</Text><Button label="Tentar novamente" onPress={onRetry} /></SafeAreaView>; }

function LoginScreen({ configured, onSignIn, onDevToken }: { configured: boolean; onSignIn: () => Promise<void>; onDevToken: (token: string) => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [devToken, setDevToken] = useState(process.env.EXPO_PUBLIC_ACCESS_TOKEN ?? "");
  async function run(action: () => Promise<void>) { setBusy(true); setError(undefined); try { await action(); } catch (cause) { setError(cause instanceof Error ? cause.message : "Não foi possível entrar."); } finally { setBusy(false); } }
  return <SafeAreaView style={styles.onboardingSafe}><ScrollView contentContainerStyle={styles.loginScroll}><View style={styles.onboardingHero}><View style={styles.logoRow}><View style={styles.logoSmall}><Text style={styles.logoSmallText}>L</Text></View><Text style={styles.logoName}>LicitaLens</Text></View><Text style={styles.onboardingTitle}>Entre para acessar seu radar comercial.</Text><Text style={styles.onboardingCopy}>Use sua conta corporativa. Depois do login, criamos ou vinculamos sua organização e assinatura.</Text></View><View style={styles.formCard}><Text style={styles.sectionEyebrow}>ACESSO</Text><Text style={styles.formTitle}>Conta da empresa</Text>{configured ? <Button label={busy ? "Conectando…" : "Entrar com Keycloak"} disabled={busy} onPress={() => void run(onSignIn)} /> : <Text style={styles.helperDark}>Configure EXPO_PUBLIC_KEYCLOAK_ISSUER e EXPO_PUBLIC_KEYCLOAK_CLIENT_ID, ou use um token de desenvolvimento abaixo.</Text>}<Field label="Token de desenvolvimento (opcional)" value={devToken} onChangeText={setDevToken} placeholder="Bearer alternativo para testes" /><Button label="Continuar com token" disabled={busy || !devToken.trim()} onPress={() => void run(() => onDevToken(devToken.trim()))} />{error && <Text style={styles.errorText}>{error}</Text>}</View></ScrollView></SafeAreaView>;
}

function OrganizationSetupScreen({ onComplete }: { onComplete: (name: string) => Promise<void> }) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  async function submit() { if (name.trim().length < 2) return; setBusy(true); setError(undefined); try { await onComplete(name.trim()); } catch (cause) { setError(cause instanceof Error ? cause.message : "Não foi possível criar a organização."); } finally { setBusy(false); } }
  return <SafeAreaView style={styles.onboardingSafe}><ScrollView contentContainerStyle={styles.loginScroll}><View style={styles.onboardingHero}><Text style={styles.onboardingTitle}>Primeiro acesso da empresa</Text><Text style={styles.onboardingCopy}>Informe o nome da organização para provisionar plano trial, membros e cobrança.</Text></View><View style={styles.formCard}><Field label="Nome da empresa" value={name} placeholder="Ex.: ACME Tecnologia Ltda." onChangeText={setName} /><Button label={busy ? "Provisionando…" : "Criar organização"} disabled={busy || name.trim().length < 2} onPress={() => void submit()} />{error && <Text style={styles.errorText}>{error}</Text>}</View></ScrollView></SafeAreaView>;
}

function Onboarding({ onComplete }: { onComplete: (profile: ProfileInput) => Promise<void> }) {
  const [draft, setDraft] = useState(initialDraft);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const valid = draft.name.trim().length > 1 && draft.description.trim().length > 9;
  async function submit() { if (!valid) return; setSaving(true); setError(undefined); try { await onComplete(toProfileInput(draft)); } catch (cause) { setError(cause instanceof Error ? cause.message : "Não foi possível criar o perfil."); } finally { setSaving(false); } }
  return <SafeAreaView style={styles.onboardingSafe}><ExpoStatusBar style="light" /><ScrollView contentContainerStyle={styles.onboardingScroll} keyboardShouldPersistTaps="handled">
    <View style={styles.onboardingHero}><View style={styles.logoRow}><View style={styles.logoSmall}><Text style={styles.logoSmallText}>L</Text></View><Text style={styles.logoName}>LicitaLens</Text></View><Text style={styles.onboardingTitle}>Encontre licitações que fazem sentido para o seu negócio.</Text><Text style={styles.onboardingCopy}>Conte o que sua empresa fornece. Nós organizamos as oportunidades e explicamos a aderência.</Text><View style={styles.steps}><Text style={styles.stepActive}>1</Text><View style={styles.stepLine} /><Text style={styles.step}>2</Text><View style={styles.stepLine} /><Text style={styles.step}>3</Text></View></View>
    <View style={styles.formCard}><Text style={styles.sectionEyebrow}>SEU PERFIL COMERCIAL</Text><Text style={styles.formTitle}>O que você fornece?</Text><Field label="Nome do perfil" value={draft.name} placeholder="Ex.: Tecnologia e informática" onChangeText={(name) => setDraft({ ...draft, name })} /><Field label="Descrição" value={draft.description} placeholder="Descreva produtos, serviços e diferenciais" multiline onChangeText={(description) => setDraft({ ...draft, description })} /><Field label="Palavras-chave" value={draft.keywords} placeholder="notebook, servidor, suporte" hint="Separe os termos por vírgula" onChangeText={(keywords) => setDraft({ ...draft, keywords })} /><Field label="Estados atendidos" value={draft.states} placeholder="PR, SC, SP" hint="Deixe vazio para atender todo o Brasil" onChangeText={(states) => setDraft({ ...draft, states })} /><View style={styles.fieldRow}><View style={styles.fieldHalf}><Field label="Valor mínimo" value={draft.minimum} placeholder="R$ 0" keyboardType="numeric" onChangeText={(minimum) => setDraft({ ...draft, minimum })} /></View><View style={styles.fieldHalf}><Field label="Valor máximo" value={draft.maximum} placeholder="R$ 500.000" keyboardType="numeric" onChangeText={(maximum) => setDraft({ ...draft, maximum })} /></View></View>{error && <Text style={styles.errorText}>{error}</Text>}<Button label={saving ? "Criando perfil…" : "Criar meu radar"} disabled={!valid || saving} onPress={() => void submit()} /><Text style={styles.privacy}>Você poderá alterar todas essas informações depois.</Text></View>
  </ScrollView></SafeAreaView>;
}

function Feed({ items, refreshing, onRefresh, onSelect }: { items: Opportunity[]; refreshing: boolean; onRefresh: () => void; onSelect: (item: Opportunity) => void }) {
  const [query, setQuery] = useState(""); const [state, setState] = useState("Todos");
  const states = ["Todos", ...Array.from(new Set(items.map((item) => item.state).filter((value): value is string => Boolean(value))))];
  const filtered = items.filter((item) => (state === "Todos" || item.state === state) && `${item.object} ${item.organization_name} ${item.municipality}`.toLocaleLowerCase("pt-BR").includes(query.trim().toLocaleLowerCase("pt-BR")));
  return <View style={styles.screen}><View style={styles.hero}><View><Text style={styles.heroEyebrow}>SEU RADAR COMERCIAL</Text><Text style={styles.heroTitle}>Boas oportunidades</Text><Text style={styles.heroCopy}>{filtered.length} licitações para avaliar</Text></View><View style={styles.avatar}><Text style={styles.avatarText}>LL</Text></View></View><View style={styles.searchWrap}><Text style={styles.searchIcon}>⌕</Text><TextInput value={query} onChangeText={setQuery} placeholder="Buscar objeto, órgão ou cidade" placeholderTextColor="#98A2B3" style={styles.searchInput} /></View><View style={styles.chipArea}><ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.chips}>{states.map((item) => <Chip key={item} label={item} active={state === item} onPress={() => setState(item)} />)}</ScrollView></View><FlatList data={filtered} keyExtractor={(item) => item.id} refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={colors.blue} />} contentContainerStyle={styles.list} ListEmptyComponent={<EmptyState title="Nenhuma oportunidade aqui" text="Tente remover os filtros ou atualizar o radar." />} renderItem={({ item }) => <OpportunityCard item={item} onPress={() => onSelect(item)} />} /></View>;
}

function OpportunityCard({ item, onPress }: { item: Opportunity; onPress: () => void }) { const days = daysUntil(item.proposal_deadline); return <Pressable style={({ pressed }) => [styles.card, pressed && styles.pressed]} onPress={onPress}><View style={styles.cardTop}><View style={styles.badge}><Text style={styles.badgeText}>{item.state || "BR"}</Text></View><Text style={[styles.deadline, days <= 3 && styles.deadlineUrgent]}>{days < 0 ? "Encerrada" : days === 0 ? "Encerra hoje" : `${days} dias restantes`}</Text></View><Text style={styles.cardTitle} numberOfLines={3}>{item.object}</Text><Text style={styles.cardOrganization} numberOfLines={1}>{item.organization_name}</Text><View style={styles.cardDivider} /><View style={styles.cardBottom}><View><Text style={styles.metaLabel}>VALOR ESTIMADO</Text><Text style={styles.cardValue}>{money(item.estimated_value_cents)}</Text></View><View style={styles.openCircle}><Text style={styles.openArrow}>→</Text></View></View></Pressable>; }

function OpportunityDetail({ item, analysis, onClose, onAnalyze }: { item?: Opportunity; analysis?: Analysis; onClose: () => void; onAnalyze: () => Promise<void> }) {
  const [analyzing, setAnalyzing] = useState(false); const [error, setError] = useState<string>();
  useEffect(() => { setError(undefined); setAnalyzing(false); }, [item]);
  if (!item) return null;
  async function run() { setAnalyzing(true); setError(undefined); try { await onAnalyze(); } catch (cause) { setError(cause instanceof Error ? cause.message : "Análise indisponível."); } finally { setAnalyzing(false); } }
  return <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}><SafeAreaView style={styles.detailSafe}><ExpoStatusBar style="dark" /><View style={styles.detailBar}><Pressable onPress={onClose} style={styles.backButton}><Text style={styles.backText}>‹</Text></Pressable><Text style={styles.detailBarTitle}>Detalhes</Text><View style={styles.backButton} /></View><ScrollView contentContainerStyle={styles.detailContent}><View style={styles.detailBadges}><View style={styles.badge}><Text style={styles.badgeText}>{item.state || "BR"}</Text></View><Text style={styles.source}>{item.source.toUpperCase()}</Text></View><Text style={styles.detailTitle}>{item.object}</Text><Text style={styles.detailOrganization}>{item.organization_name}</Text><View style={styles.summaryGrid}><Summary label="Valor estimado" value={money(item.estimated_value_cents)} /><Summary label="Prazo" value={deadlineLabel(item.proposal_deadline)} /><Summary label="Local" value={[item.municipality, item.state].filter(Boolean).join(" · ") || "Nacional"} /><Summary label="Modalidade" value={`Código ${item.modality_code ?? "—"}`} /></View>{!analysis && <View style={styles.analysisCallout}><Text style={styles.analysisSpark}>✦</Text><Text style={styles.analysisTitle}>Entenda a aderência</Text><Text style={styles.analysisCopy}>Cruze esta oportunidade com seu perfil comercial. A análise é informativa e não substitui o edital.</Text><Button label={analyzing ? "Analisando…" : "Analisar aderência"} disabled={analyzing} onPress={() => void run()} /></View>}{analysis && !analysis.matched && <EmptyState title="Fora do perfil" text="Esta oportunidade não passou pelos filtros objetivos do seu perfil comercial." />}{analysis?.matched && analysis.match && <AnalysisPanel analysis={analysis} />}{error && <Text style={styles.errorText}>{error}</Text>}<View style={styles.sourceCard}><Text style={styles.metaLabel}>{item.source === "demo" ? "DADOS DE DEMONSTRAÇÃO" : "FONTE E ATUALIZAÇÃO"}</Text><Text style={styles.sourceLink}>{item.source_url || "Registro público"}</Text>{item.source === "demo" && <Text style={styles.sourceWarning}>Registro ilustrativo; confirme sempre no PNCP ou na plataforma oficial.</Text>}</View><View style={styles.legalNotice}><Text style={styles.legalNoticeTitle}>Apoio à decisão comercial</Text><Text style={styles.legalNoticeText}>O score indica aderência ao perfil. Não confirma habilitação, classificação, julgamento ou chance de vitória.</Text></View></ScrollView></SafeAreaView></Modal>;
}

function AnalysisPanel({ analysis }: { analysis: Analysis }) { const match = analysis.match!; const factors = [["Semântica", match.breakdown?.semantic], ["Recência", match.breakdown?.recency], ["Faixa financeira", match.breakdown?.financial], ["Concorrência", match.breakdown?.competition]] as const; return <View style={styles.analysisResult}><View style={styles.scoreRow}><View><Text style={styles.metaLabel}>ADERÊNCIA AO PERFIL</Text><Text style={styles.scoreLabel}>Indicador comercial</Text><Text style={styles.analysisDisclaimer}>Não confirma habilitação, classificação ou chance de vitória.</Text></View><View style={styles.scoreCircle}><Text style={styles.scoreValue}>{match.score ?? 0}</Text><Text style={styles.scoreSuffix}>/100</Text></View></View>{factors.map(([label, value]) => <Factor key={label} label={label} value={value ?? 0} />)}<View style={styles.evidenceBox}><Text style={styles.evidenceTitle}>Por que combina</Text><Text style={styles.evidenceText}>{match.explanation || match.evidence?.join(" • ") || "A oportunidade atende aos critérios configurados."}</Text></View></View>; }

function ProfileScreen({ profile, onSave }: { profile: Profile; onSave: (input: ProfileInput) => Promise<Profile> }) {
  const [draft, setDraft] = useState(() => fromProfile(profile)); const [saving, setSaving] = useState(false); const [message, setMessage] = useState<string>();
  async function save() { setSaving(true); setMessage(undefined); try { await onSave(toProfileInput(draft)); setMessage("Perfil atualizado com sucesso."); } catch (cause) { setMessage(cause instanceof Error ? cause.message : "Não foi possível salvar."); } finally { setSaving(false); } }
  return <ScrollView style={styles.screen} contentContainerStyle={styles.pageContent} keyboardShouldPersistTaps="handled"><PageHeader eyebrow="RADAR COMERCIAL" title="Seu perfil" copy="Quanto mais preciso, melhores serão as oportunidades selecionadas." /><View style={styles.panel}><Field label="Nome do perfil" value={draft.name} onChangeText={(name) => setDraft({ ...draft, name })} /><Field label="O que sua empresa fornece" value={draft.description} multiline onChangeText={(description) => setDraft({ ...draft, description })} /><Field label="Palavras-chave" value={draft.keywords} hint="Separe por vírgula" onChangeText={(keywords) => setDraft({ ...draft, keywords })} /><Field label="Estados atendidos" value={draft.states} hint="Ex.: PR, SC, SP" onChangeText={(states) => setDraft({ ...draft, states })} /><View style={styles.fieldRow}><View style={styles.fieldHalf}><Field label="Valor mínimo" value={draft.minimum} keyboardType="numeric" onChangeText={(minimum) => setDraft({ ...draft, minimum })} /></View><View style={styles.fieldHalf}><Field label="Valor máximo" value={draft.maximum} keyboardType="numeric" onChangeText={(maximum) => setDraft({ ...draft, maximum })} /></View></View>{message && <Text style={message.includes("sucesso") ? styles.successText : styles.errorText}>{message}</Text>}<Button label={saving ? "Salvando…" : "Salvar alterações"} disabled={saving} onPress={() => void save()} /></View></ScrollView>;
}

function UsageScreen({ usage, history, organizationName, onUpgradeEssential, onUpgradePro, onManage }: { usage: Usage; history: SubscriptionHistoryEntry[]; organizationName: string; onUpgradeEssential: () => Promise<void>; onUpgradePro: () => Promise<void>; onManage: () => Promise<void> }) {
  return <ScrollView style={styles.screen} contentContainerStyle={styles.pageContent}><PageHeader eyebrow="ASSINATURA" title="Plano e consumo" copy="Contrate, gerencie a cobrança e acompanhe o histórico da sua organização." /><View style={styles.planCard}><View><Text style={styles.planPill}>PLANO ATUAL</Text><Text style={styles.planName}>{usage.plan === "pro" ? "Pro" : "Essencial"}</Text><Text style={styles.planCopy}>{organizationName} · checkout Stripe e portal do cliente conectados ao backend.</Text></View><Text style={styles.planStar}>✦</Text></View><View style={styles.panel}><UsageRow label="Perfis monitorados" item={usage.profiles} /><UsageRow label="Análises no mês" item={usage.ai_analyses} /><UsageRow label="Alertas de hoje" item={usage.daily_alerts} /><View style={styles.billingActions}><Button label="Assinar Essencial" onPress={() => void onUpgradeEssential()} /><Button label="Assinar Pro" onPress={() => void onUpgradePro()} /><Button label="Gerenciar cobrança" onPress={() => void onManage()} /></View></View><View style={styles.panel}><Text style={styles.sectionEyebrow}>HISTÓRICO</Text>{history.length === 0 ? <Text style={styles.settingText}>Nenhum evento de assinatura registrado ainda.</Text> : history.map((item) => <View key={item.id} style={styles.historyRow}><Text style={styles.settingTitle}>{item.plan.toUpperCase()} · {item.status}</Text><Text style={styles.settingText}>{new Date(item.recorded_at).toLocaleString("pt-BR")}{item.stripe_event_id ? ` · ${item.stripe_event_id}` : ""}</Text></View>)}</View></ScrollView>;
}

function SettingsScreen({ preferences, onChange, onSignOut, adminUrl }: { preferences: Preferences; onChange: (value: Preferences) => Promise<void>; onSignOut?: () => Promise<void>; adminUrl: string }) { const options: Array<[keyof Preferences, string, string]> = [["push", "Notificações no aplicativo", "Novas oportunidades com boa aderência"], ["email", "Resumo por e-mail", "Uma seleção diária do seu radar"], ["whatsapp", "WhatsApp", "Canal experimental, será conectado depois"], ["deadlineReminder", "Lembrete de prazo", "Aviso antes do encerramento da proposta"]]; return <ScrollView style={styles.screen} contentContainerStyle={styles.pageContent}><PageHeader eyebrow="PREFERÊNCIAS" title="Alertas e conta" copy="Escolha como deseja acompanhar novas oportunidades." /><View style={styles.panel}>{options.map(([key, title, copy], index) => <View key={key} style={[styles.settingRow, index > 0 && styles.settingBorder]}><View style={styles.settingCopy}><Text style={styles.settingTitle}>{title}</Text><Text style={styles.settingText}>{copy}</Text></View><Switch value={preferences[key]} onValueChange={(value) => void onChange({ ...preferences, [key]: value })} trackColor={{ false: "#D0D5DD", true: "#9DBAFD" }} thumbColor={preferences[key] ? colors.blue : "#F2F4F7"} /></View>)}</View><View style={styles.infoCard}><Text style={styles.infoTitle}>Operação comercial</Text><Text style={styles.infoText}>Painel interno em {adminUrl}. Em produção, proteja com ADMIN_API_KEY.</Text>{onSignOut && <View style={{ marginTop: 14 }}><Button label="Sair da conta" onPress={() => void onSignOut()} /></View>}</View></ScrollView>; }

function Navigation({ active, onChange, desktop = false }: { active: Tab; onChange: (tab: Tab) => void; desktop?: boolean }) { const items: Array<[Tab, string, string]> = [["feed", "⌂", "Oportunidades"], ["profile", "◎", "Perfil"], ["usage", "◫", "Plano"], ["settings", "⚙", "Ajustes"]]; return <View style={desktop ? styles.sidebar : styles.bottomNav}>{desktop && <View style={styles.sidebarBrand}><View style={styles.logoSmall}><Text style={styles.logoSmallText}>L</Text></View><Text style={styles.sidebarBrandText}>LicitaLens</Text></View>}{items.map(([key, icon, label]) => <Pressable key={key} onPress={() => onChange(key)} style={[desktop ? styles.sideNavItem : styles.navItem, active === key && desktop && styles.sideNavActive]}><Text style={[desktop ? styles.sideNavIcon : styles.navIcon, active === key && styles.navActive]}>{icon}</Text><Text style={[desktop ? styles.sideNavLabel : styles.navLabel, active === key && styles.navActive]}>{label}</Text></Pressable>)}</View>; }
function PageHeader({ eyebrow, title, copy }: { eyebrow: string; title: string; copy: string }) { return <View style={styles.pageHeader}><Text style={styles.pageEyebrow}>{eyebrow}</Text><Text style={styles.pageTitle}>{title}</Text><Text style={styles.pageCopy}>{copy}</Text></View>; }
function Button({ label, onPress, disabled = false }: { label: string; onPress: () => void; disabled?: boolean }) { return <Pressable onPress={onPress} disabled={disabled} style={({ pressed }) => [styles.button, (pressed || disabled) && styles.buttonMuted]}><Text style={styles.buttonText}>{label}</Text></Pressable>; }
function Chip({ label, active, onPress }: { label: string; active: boolean; onPress: () => void }) { return <Pressable onPress={onPress} style={[styles.chip, active && styles.chipActive]}><Text style={[styles.chipText, active && styles.chipTextActive]}>{label}</Text></Pressable>; }
function Field(props: { label: string; value: string; onChangeText: (value: string) => void; placeholder?: string; hint?: string; multiline?: boolean; keyboardType?: "default" | "numeric" }) { return <View style={styles.field}><Text style={styles.fieldLabel}>{props.label}</Text><TextInput value={props.value} onChangeText={props.onChangeText} placeholder={props.placeholder} multiline={props.multiline} keyboardType={props.keyboardType} placeholderTextColor="#98A2B3" style={[styles.input, props.multiline && styles.inputMultiline]} textAlignVertical={props.multiline ? "top" : "center"} />{props.hint && <Text style={styles.fieldHint}>{props.hint}</Text>}</View>; }
function Summary({ label, value }: { label: string; value: string }) { return <View style={styles.summary}><Text style={styles.metaLabel}>{label.toUpperCase()}</Text><Text style={styles.summaryValue}>{value}</Text></View>; }
function Factor({ label, value }: { label: string; value: number }) { return <View style={styles.factor}><View style={styles.factorTop}><Text style={styles.factorLabel}>{label}</Text><Text style={styles.factorValue}>{Math.round(value * 100)}%</Text></View><View style={styles.track}><View style={[styles.fill, { width: `${Math.round(value * 100)}%` }]} /></View></View>; }
function UsageRow({ label, item }: { label: string; item: { used: number; limit: number } }) { const ratio = item.limit ? Math.min(1, item.used / item.limit) : 0; return <View style={styles.usageRow}><View style={styles.factorTop}><Text style={styles.usageLabel}>{label}</Text><Text style={styles.usageValue}>{item.used} de {item.limit}</Text></View><View style={styles.track}><View style={[styles.fill, { width: `${ratio * 100}%` }]} /></View></View>; }
function EmptyState({ title, text }: { title: string; text: string }) { return <View style={styles.empty}><View style={styles.emptyLens}><Text style={styles.emptyLensText}>⌕</Text></View><Text style={styles.emptyTitle}>{title}</Text><Text style={styles.emptyText}>{text}</Text></View>; }

function toProfileInput(draft: Draft): ProfileInput { return { name: draft.name.trim(), description: draft.description.trim(), keywords: splitList(draft.keywords), states: splitList(draft.states).map((value) => value.toUpperCase()), minimum_value_cents: parseCurrency(draft.minimum), maximum_value_cents: parseCurrency(draft.maximum) }; }
function fromProfile(profile: Profile): Draft { return { name: profile.name, description: profile.description, keywords: profile.keywords?.join(", ") ?? "", states: profile.states?.join(", ") ?? "", minimum: formatInput(profile.minimum_value_cents), maximum: formatInput(profile.maximum_value_cents) }; }
function splitList(value: string) { return value.split(",").map((item) => item.trim()).filter(Boolean); }
function parseCurrency(value: string) { const normalized = value.replace(/\D/g, ""); return normalized ? Number(normalized) * 100 : 0; }
function formatInput(cents?: number) { return cents ? String(Math.round(cents / 100)) : ""; }
function money(cents?: number) { return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL", maximumFractionDigits: 0 }).format((cents ?? 0) / 100); }
function daysUntil(value?: string) { if (!value) return 0; return Math.ceil((new Date(value).getTime() - Date.now()) / 86_400_000); }
function deadlineLabel(value?: string) { if (!value) return "Não informado"; const days = daysUntil(value); return days < 0 ? "Encerrada" : days === 0 ? "Encerra hoje" : `${days} dias`; }

const shadow = Platform.select({ web: { boxShadow: "0 8px 30px rgba(16,35,63,0.08)" }, default: { shadowColor: colors.navy, shadowOpacity: 0.08, shadowRadius: 18, shadowOffset: { width: 0, height: 8 }, elevation: 3 } });
const styles = StyleSheet.create({
  app: { flex: 1, backgroundColor: colors.canvas, paddingTop: Platform.OS === "android" ? StatusBar.currentHeight : 0 }, shell: { flex: 1 }, shellDesktop: { flexDirection: "row", maxWidth: 1440, width: "100%", alignSelf: "center" }, main: { flex: 1 }, screen: { flex: 1, backgroundColor: colors.canvas }, centered: { flex: 1, alignItems: "center", justifyContent: "center", padding: 28, backgroundColor: colors.canvas },
  brandMark: { width: 58, height: 58, borderRadius: 18, backgroundColor: colors.blue, alignItems: "center", justifyContent: "center" }, brandMarkText: { color: "white", fontWeight: "900", fontSize: 28 }, brandName: { fontSize: 24, color: colors.navy, fontWeight: "800", marginTop: 12 }, noticeIcon: { width: 52, height: 52, borderRadius: 26, backgroundColor: colors.warningSoft, alignItems: "center", justifyContent: "center" }, noticeIconText: { color: colors.warning, fontWeight: "900", fontSize: 24 }, helper: { color: colors.muted, textAlign: "center", fontSize: 12, marginBottom: 24 },
  onboardingSafe: { flex: 1, backgroundColor: colors.navy }, onboardingScroll: { flexGrow: 1, flexDirection: Platform.OS === "web" ? "row" : "column", backgroundColor: colors.canvas }, loginScroll: { flexGrow: 1, backgroundColor: colors.canvas }, onboardingHero: { backgroundColor: colors.navy, padding: 28, paddingTop: 38, flex: Platform.OS === "web" ? 1 : undefined, justifyContent: "center" }, logoRow: { flexDirection: "row", alignItems: "center", gap: 10, marginBottom: 42 }, logoSmall: { width: 34, height: 34, borderRadius: 10, backgroundColor: colors.blue, alignItems: "center", justifyContent: "center" }, logoSmallText: { color: "white", fontSize: 17, fontWeight: "900" }, logoName: { color: "white", fontSize: 20, fontWeight: "800" }, onboardingTitle: { color: "white", fontSize: 34, lineHeight: 40, fontWeight: "800", maxWidth: 560 }, onboardingCopy: { color: "#C8D6E8", fontSize: 16, lineHeight: 24, marginTop: 18, maxWidth: 540 }, steps: { flexDirection: "row", alignItems: "center", marginTop: 38, width: 164 }, step: { width: 28, height: 28, borderRadius: 14, color: "#8FA3BA", borderWidth: 1, borderColor: "#58708A", textAlign: "center", lineHeight: 26, fontWeight: "700" }, stepActive: { width: 28, height: 28, borderRadius: 14, overflow: "hidden", color: "white", backgroundColor: colors.blue, textAlign: "center", lineHeight: 28, fontWeight: "800" }, stepLine: { flex: 1, height: 1, backgroundColor: "#58708A" }, formCard: { flex: Platform.OS === "web" ? 1 : undefined, backgroundColor: colors.surface, padding: 28, paddingTop: 34, justifyContent: "center" }, sectionEyebrow: { color: colors.blue, fontSize: 11, letterSpacing: 1.4, fontWeight: "800" }, formTitle: { color: colors.ink, fontSize: 27, fontWeight: "800", marginTop: 7, marginBottom: 24 },
  field: { marginBottom: 17 }, fieldLabel: { color: colors.ink, fontSize: 13, fontWeight: "700", marginBottom: 7 }, input: { minHeight: 48, borderWidth: 1, borderColor: colors.border, backgroundColor: "#FBFCFD", borderRadius: 12, paddingHorizontal: 14, color: colors.ink, fontSize: 15 }, inputMultiline: { height: 92, paddingTop: 13 }, fieldHint: { color: colors.muted, fontSize: 11, marginTop: 5 }, fieldRow: { flexDirection: "row", gap: 12 }, fieldHalf: { flex: 1 }, button: { minHeight: 50, backgroundColor: colors.blue, borderRadius: 13, alignItems: "center", justifyContent: "center", paddingHorizontal: 24, marginTop: 4 }, buttonMuted: { opacity: 0.55 }, buttonText: { color: "white", fontSize: 15, fontWeight: "800" }, privacy: { color: colors.muted, textAlign: "center", fontSize: 11, marginTop: 14 }, helperDark: { color: colors.muted, fontSize: 13, lineHeight: 19, marginBottom: 12 }, errorText: { color: colors.danger, fontSize: 13, marginBottom: 12 }, successText: { color: colors.success, fontSize: 13, marginBottom: 12 },
  hero: { backgroundColor: colors.navy, paddingHorizontal: 22, paddingTop: 30, paddingBottom: 46, flexDirection: "row", justifyContent: "space-between", alignItems: "flex-start" }, heroEyebrow: { color: "#8EB1DB", fontSize: 10, letterSpacing: 1.2, fontWeight: "800" }, heroTitle: { color: "white", fontSize: 28, fontWeight: "800", marginTop: 8 }, heroCopy: { color: "#C8D6E8", fontSize: 14, marginTop: 6 }, avatar: { width: 40, height: 40, borderRadius: 20, backgroundColor: colors.cyan, alignItems: "center", justifyContent: "center" }, avatarText: { color: "white", fontSize: 12, fontWeight: "800" }, searchWrap: { marginHorizontal: 18, marginTop: -24, height: 50, backgroundColor: "white", borderRadius: 14, flexDirection: "row", alignItems: "center", paddingHorizontal: 14, ...shadow }, searchIcon: { color: colors.muted, fontSize: 24, marginRight: 8 }, searchInput: { flex: 1, color: colors.ink, fontSize: 14, height: 48 }, chipArea: { height: 64 }, chips: { paddingHorizontal: 18, paddingVertical: 14, gap: 8 }, chip: { height: 34, paddingHorizontal: 15, borderRadius: 17, borderWidth: 1, borderColor: colors.border, backgroundColor: "white", alignItems: "center", justifyContent: "center" }, chipActive: { backgroundColor: colors.navy, borderColor: colors.navy }, chipText: { color: colors.muted, fontWeight: "700", fontSize: 12 }, chipTextActive: { color: "white" },
  list: { paddingHorizontal: 18, paddingBottom: 110, gap: 13, maxWidth: 900, width: "100%", alignSelf: "center" }, card: { backgroundColor: "white", borderRadius: 18, padding: 18, borderWidth: 1, borderColor: "#E9EDF2", ...shadow }, pressed: { opacity: 0.82, transform: [{ scale: 0.995 }] }, cardTop: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" }, badge: { backgroundColor: colors.blueSoft, paddingVertical: 5, paddingHorizontal: 10, borderRadius: 8 }, badgeText: { color: colors.blue, fontSize: 11, fontWeight: "800" }, deadline: { color: colors.success, fontSize: 11, fontWeight: "700" }, deadlineUrgent: { color: colors.warning }, cardTitle: { color: colors.ink, fontWeight: "700", fontSize: 17, lineHeight: 23, marginTop: 15 }, cardOrganization: { color: colors.muted, fontSize: 12, marginTop: 9 }, cardDivider: { height: 1, backgroundColor: "#EEF1F4", marginVertical: 16 }, cardBottom: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" }, metaLabel: { color: "#8993A4", fontSize: 9, fontWeight: "800", letterSpacing: 0.8 }, cardValue: { color: colors.navy, fontSize: 18, fontWeight: "800", marginTop: 4 }, openCircle: { width: 34, height: 34, borderRadius: 17, backgroundColor: colors.blueSoft, alignItems: "center", justifyContent: "center" }, openArrow: { color: colors.blue, fontSize: 18, fontWeight: "700" },
  bottomNav: { minHeight: 72, paddingBottom: Platform.OS === "ios" ? 8 : 0, backgroundColor: "white", borderTopWidth: 1, borderTopColor: colors.border, flexDirection: "row", alignItems: "center" }, navItem: { flex: 1, alignItems: "center", justifyContent: "center", gap: 3 }, navIcon: { color: "#98A2B3", fontSize: 20 }, navLabel: { color: "#98A2B3", fontSize: 10, fontWeight: "700" }, navActive: { color: colors.blue }, sidebar: { width: 238, padding: 22, backgroundColor: colors.navy, gap: 8 }, sidebarBrand: { flexDirection: "row", alignItems: "center", gap: 10, marginBottom: 34 }, sidebarBrandText: { color: "white", fontSize: 19, fontWeight: "800" }, sideNavItem: { minHeight: 48, borderRadius: 12, flexDirection: "row", alignItems: "center", paddingHorizontal: 14, gap: 12 }, sideNavActive: { backgroundColor: colors.navySoft }, sideNavIcon: { color: "#9DB0C6", fontSize: 19, width: 22 }, sideNavLabel: { color: "#B5C2D1", fontWeight: "700", fontSize: 14 },
  pageContent: { padding: 22, paddingBottom: 110, maxWidth: 860, width: "100%", alignSelf: "center" }, pageHeader: { marginBottom: 24 }, pageEyebrow: { color: colors.blue, fontWeight: "800", fontSize: 10, letterSpacing: 1.3 }, pageTitle: { color: colors.ink, fontSize: 29, fontWeight: "800", marginTop: 7 }, pageCopy: { color: colors.muted, fontSize: 14, lineHeight: 20, marginTop: 6 }, panel: { backgroundColor: "white", borderRadius: 18, borderWidth: 1, borderColor: colors.border, padding: 20, ...shadow },
  detailSafe: { flex: 1, backgroundColor: colors.surface }, detailBar: { height: 58, borderBottomWidth: 1, borderBottomColor: colors.border, flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: 16 }, backButton: { width: 38, height: 38, alignItems: "center", justifyContent: "center" }, backText: { color: colors.ink, fontSize: 36, lineHeight: 36 }, detailBarTitle: { color: colors.ink, fontWeight: "700", fontSize: 15 }, detailContent: { padding: 22, paddingBottom: 50, maxWidth: 760, width: "100%", alignSelf: "center" }, detailBadges: { flexDirection: "row", alignItems: "center", gap: 10 }, source: { color: colors.muted, fontSize: 10, fontWeight: "800", letterSpacing: 0.8 }, detailTitle: { color: colors.ink, fontSize: 25, lineHeight: 32, fontWeight: "800", marginTop: 18 }, detailOrganization: { color: colors.muted, fontSize: 14, marginTop: 10 }, summaryGrid: { flexDirection: "row", flexWrap: "wrap", gap: 10, marginTop: 24 }, summary: { width: "48%", minHeight: 78, borderRadius: 13, backgroundColor: colors.canvas, padding: 13 }, summaryValue: { color: colors.ink, fontSize: 14, lineHeight: 19, fontWeight: "700", marginTop: 7 }, analysisCallout: { backgroundColor: colors.navy, borderRadius: 18, padding: 22, marginTop: 24 }, analysisSpark: { color: "#8DB1FF", fontSize: 25 }, analysisTitle: { color: "white", fontWeight: "800", fontSize: 21, marginTop: 9 }, analysisCopy: { color: "#C8D6E8", lineHeight: 20, fontSize: 13, marginTop: 7, marginBottom: 16 }, analysisResult: { backgroundColor: "white", borderRadius: 18, borderWidth: 1, borderColor: colors.border, padding: 20, marginTop: 24, ...shadow }, scoreRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginBottom: 20 }, scoreLabel: { color: colors.success, fontSize: 20, fontWeight: "800", marginTop: 5 }, analysisDisclaimer: { color: colors.muted, fontSize: 10, lineHeight: 14, marginTop: 4, maxWidth: 210 }, scoreCircle: { width: 68, height: 68, borderRadius: 34, borderWidth: 6, borderColor: colors.success, alignItems: "center", justifyContent: "center" }, scoreValue: { color: colors.ink, fontSize: 20, fontWeight: "900", lineHeight: 22 }, scoreSuffix: { color: colors.muted, fontSize: 8 }, factor: { marginTop: 14 }, factorTop: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" }, factorLabel: { color: colors.ink, fontSize: 12, fontWeight: "600" }, factorValue: { color: colors.muted, fontSize: 11, fontWeight: "700" }, track: { height: 7, borderRadius: 4, backgroundColor: "#E8EDF3", marginTop: 7, overflow: "hidden" }, fill: { height: 7, borderRadius: 4, backgroundColor: colors.blue }, evidenceBox: { marginTop: 22, padding: 16, borderRadius: 13, backgroundColor: colors.successSoft }, evidenceTitle: { color: colors.success, fontWeight: "800", fontSize: 13 }, evidenceText: { color: "#356657", fontSize: 12, lineHeight: 19, marginTop: 7 }, sourceCard: { borderTopWidth: 1, borderTopColor: colors.border, marginTop: 28, paddingTop: 18 }, sourceLink: { color: colors.blue, fontSize: 12, marginTop: 7 }, sourceWarning: { color: "#9A5B00", fontSize: 11, lineHeight: 15, marginTop: 5 }, legalNotice: { backgroundColor: "#FFF9ED", borderRadius: 14, borderWidth: 1, borderColor: "#F3D79B", padding: 12, marginTop: 18 }, legalNoticeTitle: { color: "#8A5A00", fontSize: 11, fontWeight: "800" }, legalNoticeText: { color: "#735F37", fontSize: 11, lineHeight: 16, marginTop: 3 },
  planCard: { backgroundColor: colors.navy, borderRadius: 19, padding: 22, marginBottom: 16, flexDirection: "row", justifyContent: "space-between", ...shadow }, planPill: { color: "#8DB1FF", fontSize: 9, fontWeight: "800", letterSpacing: 1 }, planName: { color: "white", fontSize: 30, fontWeight: "900", marginTop: 8 }, planCopy: { color: "#C8D6E8", fontSize: 12, lineHeight: 18, marginTop: 6, maxWidth: 440 }, planStar: { color: "#8DB1FF", fontSize: 34 }, billingActions: { gap: 10, marginTop: 18 }, historyRow: { paddingVertical: 12, borderTopWidth: 1, borderTopColor: colors.border }, usageRow: { paddingVertical: 15 }, usageLabel: { color: colors.ink, fontSize: 14, fontWeight: "700" }, usageValue: { color: colors.muted, fontSize: 12 }, settingRow: { minHeight: 76, flexDirection: "row", alignItems: "center", justifyContent: "space-between" }, settingBorder: { borderTopWidth: 1, borderTopColor: colors.border }, settingCopy: { flex: 1, paddingRight: 18 }, settingTitle: { color: colors.ink, fontSize: 14, fontWeight: "700" }, settingText: { color: colors.muted, fontSize: 12, lineHeight: 17, marginTop: 4 }, infoCard: { padding: 18, borderRadius: 16, backgroundColor: colors.blueSoft, marginTop: 16 }, infoTitle: { color: colors.blue, fontWeight: "800", fontSize: 13 }, infoText: { color: "#486284", lineHeight: 18, fontSize: 12, marginTop: 5 }, empty: { padding: 42, alignItems: "center", justifyContent: "center" }, emptyLens: { width: 48, height: 48, borderRadius: 24, backgroundColor: colors.blueSoft, alignItems: "center", justifyContent: "center" }, emptyLensText: { color: colors.blue, fontSize: 24 }, emptyTitle: { color: colors.ink, fontWeight: "800", fontSize: 18, marginTop: 14, textAlign: "center" }, emptyText: { color: colors.muted, fontSize: 13, lineHeight: 19, marginTop: 6, marginBottom: 20, textAlign: "center", maxWidth: 420 },
});
