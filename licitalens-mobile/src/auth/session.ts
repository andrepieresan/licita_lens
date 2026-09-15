import * as AuthSession from "expo-auth-session";
import * as WebBrowser from "expo-web-browser";

WebBrowser.maybeCompleteAuthSession();

const issuer = process.env.EXPO_PUBLIC_KEYCLOAK_ISSUER?.replace(/\/$/, "");
const clientId = process.env.EXPO_PUBLIC_KEYCLOAK_CLIENT_ID ?? "licitalens-mobile";

export type AuthSessionResult = {
  accessToken: string;
  refreshToken?: string;
  expiresAt?: number;
};

export function authConfigured() {
  return Boolean(issuer && clientId);
}

export async function signInWithKeycloak(): Promise<AuthSessionResult> {
  if (!issuer) {
    throw new Error("EXPO_PUBLIC_KEYCLOAK_ISSUER não configurado.");
  }
  const discovery = await AuthSession.fetchDiscoveryAsync(issuer);
  const redirectUri = AuthSession.makeRedirectUri({ scheme: "licitalens" });
  const request = new AuthSession.AuthRequest({
    clientId,
    redirectUri,
    scopes: ["openid", "profile", "email"],
    responseType: AuthSession.ResponseType.Code,
    usePKCE: true,
  });
  await request.makeAuthUrlAsync(discovery);
  const result = await request.promptAsync(discovery);
  if (result.type !== "success" || !result.params.code) {
    throw new Error("Login cancelado ou não concluído.");
  }
  const token = await AuthSession.exchangeCodeAsync(
    {
      clientId,
      code: result.params.code,
      redirectUri,
      extraParams: { code_verifier: request.codeVerifier ?? "" },
    },
    discovery,
  );
  if (!token.accessToken) {
    throw new Error("Keycloak não retornou access token.");
  }
  return {
    accessToken: token.accessToken,
    refreshToken: token.refreshToken,
    expiresAt: token.expiresIn ? Date.now() + token.expiresIn * 1000 : undefined,
  };
}

export async function signOutKeycloak() {
  await WebBrowser.coolDownAsync();
}
