// SSH Keys management
import { sshKeyClient } from "./client";

export async function listSSHKeys(accountId: string) {
  const response = await sshKeyClient.listSshKeys({ accountId });
  return response.sshKeys;
}

export async function createSSHKey(accountId: string, publicKey: string, name?: string) {
  const response = await sshKeyClient.createSshKey({
    accountId,
    publicKey,
    name,
  });
  return response.sshKey;
}

export async function deleteSSHKey(accountId: string, keyId: string) {
  await sshKeyClient.deleteSshKey({ accountId, keyId });
}
