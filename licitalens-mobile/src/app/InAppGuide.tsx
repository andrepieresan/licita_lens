import { useEffect, useMemo, useState } from "react";
import { Modal, Pressable, SafeAreaView, ScrollView, StyleSheet, Text, useWindowDimensions, View } from "react-native";
import type { GuideProgress } from "../guideProgress";
import { colors } from "../theme";

export type GuideTab = "dashboard" | "radar" | "profile" | "plan" | "settings";
export type GuidePhase = "welcome" | "login" | "signup" | "activate" | "onboarding" | "app";

export type PlaybookStep = {
  id: string;
  title: string;
  now: string;
  detail: string;
  tab?: GuideTab;
  phase?: GuidePhase;
};

export const AUTH_PLAYBOOK: PlaybookStep[] = [
  {
    id: "auth-1",
    title: "Entenda o fluxo",
    now: "Leia a sequência abaixo — você vai repetir isso no app.",
    detail: "Conta → trial → perfil inicial → radar → pipeline → alertas.",
  },
  {
    id: "auth-2",
    title: "Crie sua conta",
    now: "Toque em Criar conta e preencha e-mail, senha e nome da empresa.",
    detail: "Senha com pelo menos 8 caracteres. Use um e-mail real se for testar alertas.",
    phase: "signup",
  },
  {
    id: "auth-3",
    title: "Ative o trial",
    now: "Na tela de assinatura, escolha Continuar com trial.",
    detail: "Só use Stripe se o backend tiver cobrança configurada.",
    phase: "activate",
  },
  {
    id: "auth-4",
    title: "Perfil inicial",
    now: "Descreva o que sua empresa vende e toque Ir para o painel.",
    detail: "Depois você ajusta palavras-chave e estados na aba Perfil.",
    phase: "onboarding",
  },
];

export const APP_PLAYBOOK: PlaybookStep[] = [
  {
    id: "app-1",
    title: "Ajuste o perfil comercial",
    now: "Abra Perfil, preencha palavras-chave e UFs, depois Salvar perfil.",
    detail: "Ex.: keywords notebooks, informática · estados PR, SC",
    tab: "profile",
  },
  {
    id: "app-alert",
    title: "Crie seu alerta",
    now: "No Radar, toque em Criar ou Editar no alerta inteligente.",
    detail: "Defina palavras-chave, estados e se deseja receber o resumo por e-mail.",
    tab: "radar",
  },
  {
    id: "app-2",
    title: "Abra uma licitação",
    now: "Vá em Licitações, toque em um item e leia valor e prazo.",
    detail: "Puxe a lista para baixo para atualizar o radar.",
    tab: "radar",
  },
  {
    id: "app-3",
    title: "Analise a aderência",
    now: "No detalhe da licitação, toque Analisar aderência.",
    detail: "Veja o score e a explicação em relação ao seu perfil.",
    tab: "radar",
  },
  {
    id: "app-4",
    title: "Leve para o pipeline",
    now: "Toque Adicionar ao pipeline — você será levado ao funil.",
    detail: "O card fica ligado à licitação (Ver licitação no card).",
    tab: "radar",
  },
  {
    id: "app-5",
    title: "Registre follow-up",
    now: "No Pipeline, abra o card, escreva uma nota e Salvar follow-up.",
    detail: "Opcional: Avançar etapa ou Marcar perdido.",
    tab: "dashboard",
  },
  {
    id: "app-6",
    title: "Configure alertas",
    now: "Em Conta, ligue e-mail (e push no celular, se usar).",
    detail: "As preferências sincronizam com o servidor quando a API está no ar.",
    tab: "settings",
  },
];

type Props = {
  visible: boolean;
  onClose: () => void;
  mode: "auth" | "app";
  progress: GuideProgress;
  onProgressChange: (next: GuideProgress) => void;
  onGoToTab: (tab: GuideTab) => void;
  onGoToPhase: (phase: GuidePhase) => void;
};

export function InAppGuide({ visible, onClose, mode, progress, onProgressChange, onGoToTab, onGoToPhase }: Props) {
  const desktop = useWindowDimensions().width >= 900;
  const steps = useMemo(() => (mode === "auth" ? AUTH_PLAYBOOK : APP_PLAYBOOK), [mode]);
  const [index, setIndex] = useState(progress.stepIndex);

  useEffect(() => {
    if (visible) setIndex(progress.stepIndex);
  }, [visible, progress.stepIndex]);

  const step = steps[index] ?? steps[0]!;
  const total = steps.length;

  const completedSet = useMemo(() => new Set(progress.completed), [progress.completed]);

  const persist = (nextIndex: number, markId?: string) => {
    const completed = markId && !completedSet.has(markId) ? [...progress.completed, markId] : progress.completed;
    const next: GuideProgress = {
      active: true,
      stepIndex: Math.min(nextIndex, total - 1),
      completed,
    };
    onProgressChange(next);
    setIndex(next.stepIndex);
  };

  const close = () => {
    onClose();
  };

  const goNext = () => {
    persist(Math.min(index + 1, total - 1), step.id);
    if (index >= total - 1) {
      onProgressChange({ ...progress, active: false, stepIndex: index });
      onClose();
    }
  };

  const goBack = () => {
    const prev = Math.max(0, index - 1);
    persist(prev);
  };

  const goDo = () => {
    onProgressChange({ ...progress, active: true, stepIndex: index });
    if (step.tab) onGoToTab(step.tab);
    if (step.phase) onGoToPhase(step.phase);
    onClose();
  };

  const markDone = () => {
    persist(index + 1, step.id);
    if (index >= total - 1) {
      onProgressChange({ active: false, stepIndex: 0, completed: [...new Set([...progress.completed, step.id])] });
      onClose();
    }
  };

  return (
    <Modal
      visible={visible}
      animationType={desktop ? "fade" : "slide"}
      transparent={desktop}
      presentationStyle={desktop ? "overFullScreen" : "pageSheet"}
      onRequestClose={close}
    >
      <View style={[styles.overlay, desktop && styles.overlayDesktop]}>
        <SafeAreaView style={[styles.safe, desktop && styles.safeDesktop]}>
        <View style={[styles.topBar, desktop && styles.topBarDesktop]}>
          <Pressable onPress={close} hitSlop={12}>
            <Text style={styles.close}>Fechar</Text>
          </Pressable>
          <Text style={styles.headerTitle}>Roteiro passo a passo</Text>
          <Text style={styles.counter}>
            {index + 1}/{total}
          </Text>
        </View>

        <ScrollView contentContainerStyle={[styles.scroll, desktop && styles.scrollDesktop]} showsVerticalScrollIndicator={false}>
          <View style={[styles.guideLayout, desktop && styles.guideLayoutDesktop]}>
          <View style={styles.stepsColumn}>
            <View style={styles.progressHeading}>
              <View><Text style={styles.progressEyebrow}>SEU PROGRESSO</Text><Text style={styles.progressTitle}>Avance no seu ritmo</Text></View>
              <Text style={styles.progressCount}>{completedSet.size}/{total} concluídos</Text>
            </View>
            <View style={[styles.stepsGrid, desktop && styles.stepsGridDesktop]}>
            {steps.map((item, i) => {
            const done = completedSet.has(item.id);
            const current = i === index;
            return (
              <Pressable
                key={item.id}
                onPress={() => persist(i)}
                style={[styles.timelineRow, desktop && styles.timelineRowDesktop, current && styles.timelineRowCurrent, done && styles.timelineRowDone]}
              >
                <View style={[styles.stepBadge, done && styles.stepBadgeDone, current && styles.stepBadgeCurrent]}>
                  <Text style={[styles.stepBadgeText, (done || current) && styles.stepBadgeTextActive]}>{done ? "✓" : i + 1}</Text>
                </View>
                <View style={styles.timelineCopy}>
                  <Text style={[styles.timelineTitle, current && styles.timelineTitleCurrent]}>{item.title}</Text>
                  {current ? <Text style={styles.timelineHint}>Você está aqui</Text> : null}
                </View>
              </Pressable>
            );
            })}
            </View>
          </View>

          <View style={[styles.guideSummary, desktop && styles.guideSummaryDesktop]}>
            <View style={styles.nowBox}>
              <View style={styles.nowHeader}><Text style={styles.nowLabel}>AGORA</Text><View style={styles.nowLive}><View style={styles.nowLiveDot} /><Text style={styles.nowLiveText}>Etapa atual</Text></View></View>
              <Text style={styles.nowText}>{step.now}</Text>
            </View>
            <Text style={styles.detail}>{step.detail}</Text>
          </View>
          </View>
        </ScrollView>

        <View style={[styles.footer, desktop && styles.footerDesktop]}>
          <Pressable onPress={goBack} disabled={index === 0} style={[styles.secondaryBtn, desktop && styles.secondaryBtnDesktop, index === 0 && styles.disabled]}>
            <Text style={styles.secondaryText}>Anterior</Text>
          </Pressable>
          <View style={[styles.footerMain, desktop && styles.footerMainDesktop]}>
            {(step.tab || step.phase) && (
              <Pressable onPress={goDo} style={[styles.primaryBtn, desktop && styles.primaryBtnDesktop]}>
                <Text style={styles.primaryText}>{step.tab ? "Ir fazer agora" : "Abrir tela"}</Text>
              </Pressable>
            )}
              <Pressable onPress={markDone} style={[styles.ghostBtn, desktop && styles.ghostBtnDesktop]}>
              <Text style={styles.ghostText}>{index >= total - 1 ? "Concluir roteiro" : "Já fiz — próximo"}</Text>
            </Pressable>
            {!step.tab && !step.phase && (
              <Pressable onPress={goNext} style={[styles.primaryBtn, desktop && styles.primaryBtnDesktop]}>
                <Text style={styles.primaryText}>Próximo</Text>
              </Pressable>
            )}
          </View>
        </View>
        </SafeAreaView>
      </View>
    </Modal>
  );
}

type BarProps = {
  visible: boolean;
  step: PlaybookStep;
  index: number;
  total: number;
  onOpen: () => void;
  onDismiss: () => void;
};

export function PlaybookBar({ visible, step, index, total, onOpen, onDismiss }: BarProps) {
  if (!visible) return null;
  return (
    <Pressable style={styles.bar} onPress={onOpen}>
      <View style={styles.barCopy}>
        <Text style={styles.barEyebrow}>
          GUIA · {index + 1}/{total}
        </Text>
        <Text style={styles.barTitle} numberOfLines={1}>
          {step.title}
        </Text>
      </View>
      <Pressable onPress={onDismiss} hitSlop={8} style={styles.barDismiss}>
        <Text style={styles.barDismissText}>×</Text>
      </Pressable>
    </Pressable>
  );
}

export function stepForIndex(mode: "auth" | "app", index: number): PlaybookStep {
  const steps = mode === "auth" ? AUTH_PLAYBOOK : APP_PLAYBOOK;
  return steps[Math.min(index, steps.length - 1)] ?? steps[0]!;
}

export function advanceOnStepId(progress: GuideProgress, stepId: string, mode: "auth" | "app"): GuideProgress {
  const steps = mode === "auth" ? AUTH_PLAYBOOK : APP_PLAYBOOK;
  const idx = steps.findIndex((s) => s.id === stepId);
  if (idx < 0 || !progress.active) return progress;
  if (progress.stepIndex !== idx) return progress;
  const completed = progress.completed.includes(stepId) ? progress.completed : [...progress.completed, stepId];
  const nextIndex = idx + 1;
  if (nextIndex >= steps.length) {
    return { active: false, stepIndex: 0, completed };
  }
  return { active: true, stepIndex: nextIndex, completed };
}

const styles = StyleSheet.create({
  overlay: { flex: 1 },
  overlayDesktop: { alignItems: "center", justifyContent: "center", padding: 28, backgroundColor: "rgba(11, 27, 52, 0.56)" },
  safe: { flex: 1, backgroundColor: colors.canvas },
  safeDesktop: {
    width: "100%",
    maxWidth: 1040,
    maxHeight: "92%",
    backgroundColor: "#F4F7FB",
    borderRadius: 26,
    overflow: "hidden",
    shadowColor: "#071A35",
    shadowOpacity: 0.28,
    shadowRadius: 34,
    shadowOffset: { width: 0, height: 18 },
    elevation: 18,
  },
  topBar: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingHorizontal: 20, paddingVertical: 14, gap: 8, backgroundColor: "white", borderBottomWidth: 1, borderBottomColor: colors.border },
  topBarDesktop: { paddingHorizontal: 30, paddingVertical: 18, minHeight: 72 },
  close: { color: colors.blue, fontWeight: "800", fontSize: 15, minWidth: 52 },
  headerTitle: { flex: 1, textAlign: "center", fontWeight: "800", color: colors.ink, fontSize: 15 },
  counter: { color: colors.muted, fontWeight: "800", fontSize: 13, minWidth: 52, textAlign: "right" },
  scroll: { padding: 16, paddingBottom: 20, gap: 8 },
  scrollDesktop: { width: "100%", maxWidth: 940, alignSelf: "center", paddingHorizontal: 30, paddingTop: 24, paddingBottom: 24 },
  guideLayout: { gap: 14 },
  guideLayoutDesktop: { gap: 18 },
  stepsColumn: { gap: 10 },
  progressHeading: { flexDirection: "row", alignItems: "flex-end", justifyContent: "space-between", gap: 10, paddingHorizontal: 2, paddingBottom: 2 },
  progressEyebrow: { color: colors.blue, fontSize: 9, fontWeight: "900", letterSpacing: 1 },
  progressTitle: { color: colors.ink, fontSize: 20, fontWeight: "900", letterSpacing: -0.3, marginTop: 3 },
  progressCount: { color: colors.muted, fontSize: 11, fontWeight: "800", paddingBottom: 2 },
  stepsGrid: { gap: 8 },
  stepsGridDesktop: { flexDirection: "row", flexWrap: "wrap", gap: 12 },
  timelineRow: { flexDirection: "row", alignItems: "center", gap: 12, padding: 12, borderRadius: 14, backgroundColor: "white", borderWidth: 1, borderColor: colors.border },
  timelineRowDesktop: { width: "48.8%", minHeight: 72, paddingHorizontal: 15, borderRadius: 16 },
  timelineRowCurrent: { borderColor: colors.blue, backgroundColor: colors.blueSoft },
  timelineRowDone: { opacity: 0.85 },
  stepBadge: { width: 32, height: 32, borderRadius: 16, backgroundColor: colors.border, alignItems: "center", justifyContent: "center" },
  stepBadgeCurrent: { backgroundColor: colors.blue },
  stepBadgeDone: { backgroundColor: colors.success },
  stepBadgeText: { fontWeight: "800", color: colors.muted, fontSize: 14 },
  stepBadgeTextActive: { color: "white" },
  timelineCopy: { flex: 1, gap: 2 },
  timelineTitle: { fontWeight: "700", color: colors.ink, fontSize: 15 },
  timelineTitleCurrent: { color: colors.blue },
  timelineHint: { fontSize: 11, fontWeight: "800", color: colors.blue },
  guideSummary: { gap: 8 },
  guideSummaryDesktop: { flexDirection: "row", alignItems: "stretch", gap: 14 },
  nowBox: { flex: 1, backgroundColor: colors.navy, borderRadius: 16, padding: 16, gap: 8, marginTop: 8 },
  nowHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: 8 },
  nowLabel: { color: "#8DB1FF", fontSize: 10, fontWeight: "800", letterSpacing: 1 },
  nowLive: { flexDirection: "row", alignItems: "center", gap: 5, paddingHorizontal: 8, paddingVertical: 5, borderRadius: 8, backgroundColor: "#1D3A63" },
  nowLiveDot: { width: 6, height: 6, borderRadius: 3, backgroundColor: "#7DE0AE" },
  nowLiveText: { color: "#C4D8F5", fontSize: 9, fontWeight: "800" },
  nowText: { color: "white", fontSize: 17, fontWeight: "800", lineHeight: 24 },
  detail: { flex: 1, color: colors.muted, fontSize: 14, lineHeight: 21, paddingHorizontal: 4, paddingTop: 16 },
  footer: { padding: 16, gap: 10, borderTopWidth: 1, borderTopColor: colors.border, backgroundColor: "white" },
  footerDesktop: { paddingHorizontal: 30, paddingVertical: 16, flexDirection: "row", alignItems: "center", justifyContent: "flex-end", gap: 12 },
  footerMain: { gap: 8 },
  footerMainDesktop: { flexDirection: "row", alignItems: "center", gap: 10, minWidth: 430 },
  primaryBtn: { backgroundColor: colors.blue, borderRadius: 12, minHeight: 48, alignItems: "center", justifyContent: "center" },
  primaryBtnDesktop: { minWidth: 150, paddingHorizontal: 18 },
  primaryText: { color: "white", fontWeight: "800" },
  secondaryBtn: { borderRadius: 12, minHeight: 44, alignItems: "center", justifyContent: "center", borderWidth: 1, borderColor: colors.border },
  secondaryBtnDesktop: { minWidth: 120, paddingHorizontal: 16 },
  secondaryText: { color: colors.ink, fontWeight: "700" },
  ghostBtn: { minHeight: 44, alignItems: "center", justifyContent: "center" },
  ghostBtnDesktop: { paddingHorizontal: 8 },
  ghostText: { color: colors.blue, fontWeight: "800" },
  disabled: { opacity: 0.35 },
  bar: {
    flexDirection: "row",
    alignItems: "center",
    marginHorizontal: 16,
    marginVertical: 10,
    paddingVertical: 9,
    paddingHorizontal: 12,
    borderRadius: 14,
    backgroundColor: "white",
    borderWidth: 1,
    borderColor: "#D9E5FF",
    gap: 8,
  },
  barCopy: { flex: 1, gap: 2 },
  barEyebrow: { color: colors.blue, fontSize: 10, fontWeight: "800" },
  barTitle: { color: colors.ink, fontWeight: "700", fontSize: 13, lineHeight: 18 },
  barDismiss: { width: 28, height: 28, alignItems: "center", justifyContent: "center", borderRadius: 9, backgroundColor: colors.canvas },
  barDismissText: { color: colors.muted, fontSize: 20, fontWeight: "300", lineHeight: 20 },
});
