import AsyncStorage from "@react-native-async-storage/async-storage";

const keys = {
  onboarded: "licitalens:onboarded",
  preferences: "licitalens:preferences",
  accessToken: "licitalens:access-token",
  organizationId: "licitalens:organization-id",
  organizationName: "licitalens:organization-name",
  accountEmail: "licitalens:account-email",
};

export type Preferences = {
  push: boolean;
  email: boolean;
  whatsapp: boolean;
  deadlineReminder: boolean;
};

export const defaultPreferences: Preferences = {
  push: true,
  email: true,
  whatsapp: false,
  deadlineReminder: true,
};

export async function loadLocalState() {
  const [onboarded, preferences, accessToken, organizationId, organizationName, accountEmail] = await Promise.all([
    AsyncStorage.getItem(keys.onboarded),
    AsyncStorage.getItem(keys.preferences),
    AsyncStorage.getItem(keys.accessToken),
    AsyncStorage.getItem(keys.organizationId),
    AsyncStorage.getItem(keys.organizationName),
    AsyncStorage.getItem(keys.accountEmail),
  ]);
  return {
    onboarded: onboarded === "true",
    preferences: preferences ? ({ ...defaultPreferences, ...JSON.parse(preferences) } as Preferences) : defaultPreferences,
    accessToken: accessToken ?? process.env.EXPO_PUBLIC_ACCESS_TOKEN,
    organizationId: organizationId ?? process.env.EXPO_PUBLIC_ORGANIZATION_ID,
    organizationName: organizationName ?? "",
    accountEmail: accountEmail ?? "",
  };
}

export async function saveOnboarded(value: boolean) {
  await AsyncStorage.setItem(keys.onboarded, String(value));
}

export async function savePreferences(value: Preferences) {
  await AsyncStorage.setItem(keys.preferences, JSON.stringify(value));
}

export async function saveSession(accessToken: string, organizationId: string, organizationName = "", accountEmail = "") {
	const browser = typeof document !== "undefined";
	await Promise.all([
		browser ? AsyncStorage.removeItem(keys.accessToken) : AsyncStorage.setItem(keys.accessToken, accessToken),
    AsyncStorage.setItem(keys.organizationId, organizationId),
    AsyncStorage.setItem(keys.organizationName, organizationName),
    AsyncStorage.setItem(keys.accountEmail, accountEmail),
  ]);
}

export async function clearSession() {
  await Promise.all([
    AsyncStorage.removeItem(keys.accessToken),
    AsyncStorage.removeItem(keys.organizationId),
    AsyncStorage.removeItem(keys.organizationName),
    AsyncStorage.removeItem(keys.accountEmail),
    AsyncStorage.removeItem(keys.onboarded),
  ]);
}
