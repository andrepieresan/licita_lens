import { ReactNode } from "react";
import { StyleSheet, View } from "react-native";
import type { PlaybookStep } from "./InAppGuide";
import { colors } from "../theme";

type Tab = "overview" | "dashboard" | "radar" | "alerts" | "activity" | "profile" | "plan" | "settings";

export function SpotlightRing({ active, children }: { active: boolean; children: ReactNode }) {
  if (!active) return <>{children}</>;
  return <View style={styles.ring}>{children}</View>;
}

export function spotlightNavTab(step: PlaybookStep | null, currentTab: Tab): Tab | null {
  if (!step?.tab) return null;
  return step.tab;
}

export function spotlightSaveProfile(step: PlaybookStep | null, tab: Tab): boolean {
  return step?.id === "app-1" && tab === "profile";
}

export function spotlightRadarCard(step: PlaybookStep | null, tab: Tab, modalOpen: boolean): boolean {
  return step?.id === "app-2" && tab === "radar" && !modalOpen;
}

export function spotlightAnalyze(step: PlaybookStep | null, modalOpen: boolean): boolean {
  return step?.id === "app-3" && modalOpen;
}

export function spotlightAddPipeline(step: PlaybookStep | null, modalOpen: boolean, inPipeline: boolean): boolean {
  return step?.id === "app-4" && modalOpen && !inPipeline;
}

export function spotlightFollowUp(step: PlaybookStep | null, tab: Tab): boolean {
  return step?.id === "app-5" && tab === "dashboard";
}

export function spotlightEmailSwitch(step: PlaybookStep | null, tab: Tab): boolean {
  return step?.id === "app-6" && tab === "settings";
}

export function spotlightAuthCreateAccount(step: PlaybookStep | null, phase: string): boolean {
  return step?.id === "auth-2" && phase === "welcome";
}

export function spotlightAuthTrial(step: PlaybookStep | null, phase: string): boolean {
  return step?.id === "auth-3" && phase === "activate";
}

export function spotlightAuthOnboard(step: PlaybookStep | null, phase: string): boolean {
  return step?.id === "auth-4" && phase === "onboarding";
}

const styles = StyleSheet.create({
  ring: {
    borderWidth: 2,
    borderColor: colors.blue,
    borderRadius: 14,
    padding: 3,
    backgroundColor: "#F6F9FF",
  },
});
