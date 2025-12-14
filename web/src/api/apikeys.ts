// API Keys management
import { accountClient } from "./client";

export async function listAPIKeys() {
  const response = await accountClient.listApiKeys({});
  return response.apiKeys;
}

export async function createAPIKey(name: string, description?: string, scopes?: string[]) {
  const response = await accountClient.createApiKey({
    name,
    description: description || "",
    scopes: scopes || [],
  });
  return {
    apiKeyId: response.apiKeyId,
    apiKey: response.apiKey, // Only returned once!
    name: response.name,
    description: response.description,
    scopes: response.scopes,
    createdAt: response.createdAt,
  };
}

export async function revokeAPIKey(apiKeyId: string) {
  await accountClient.revokeApiKey({ apiKeyId });
}
