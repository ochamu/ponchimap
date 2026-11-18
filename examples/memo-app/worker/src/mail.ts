import type { Env } from "./index";

export async function sendShareMail(env: Env, to: string, body: string): Promise<void> {
  const res = await fetch("https://api.resend.com/emails", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${env.RESEND_API_KEY}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      from: env.MAIL_FROM,
      to,
      subject: "メモが共有されました",
      text: body,
    }),
  });
  if (!res.ok) {
    throw new Error(`sending the mail failed: ${res.status}`);
  }
}
