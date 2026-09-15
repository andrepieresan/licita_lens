import AsyncStorage from "@react-native-async-storage/async-storage";

const GUIDE_KEY = "licitalens:guide-progress";

export type GuideProgress = {
  active: boolean;
  stepIndex: number;
  completed: string[];
};

const defaultProgress: GuideProgress = { active: false, stepIndex: 0, completed: [] };

export async function loadGuideProgress(): Promise<GuideProgress> {
  const raw = await AsyncStorage.getItem(GUIDE_KEY);
  if (!raw) return { ...defaultProgress };
  try {
    const parsed = JSON.parse(raw) as GuideProgress;
    return {
      active: !!parsed.active,
      stepIndex: typeof parsed.stepIndex === "number" ? parsed.stepIndex : 0,
      completed: Array.isArray(parsed.completed) ? parsed.completed : [],
    };
  } catch {
    return { ...defaultProgress };
  }
}

export async function saveGuideProgress(value: GuideProgress) {
  await AsyncStorage.setItem(GUIDE_KEY, JSON.stringify(value));
}

export async function resetGuideProgress() {
  await AsyncStorage.removeItem(GUIDE_KEY);
}
