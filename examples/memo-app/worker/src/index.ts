import { sendShareMail } from "./mail";

export interface Env {
  DB: D1Database;
  RESEND_API_KEY: string;
  MAIL_FROM: string;
  APP_ORIGIN: string;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    const headers = { "Access-Control-Allow-Origin": env.APP_ORIGIN };

    if (request.method === "GET" && url.pathname === "/memos") {
      const { results } = await env.DB.prepare(
        "SELECT id, body, updated_at FROM memos ORDER BY updated_at DESC LIMIT 50",
      ).all();
      return Response.json(results, { headers });
    }

    if (request.method === "POST" && url.pathname === "/memos") {
      const { body } = await request.json<{ body: string }>();
      const id = crypto.randomUUID();
      await env.DB.prepare("INSERT INTO memos (id, body, updated_at) VALUES (?, ?, ?)")
        .bind(id, body, Date.now())
        .run();
      return Response.json({ id }, { status: 201, headers });
    }

    const share = url.pathname.match(/^\/memos\/([^/]+)\/share$/);
    if (request.method === "POST" && share) {
      const { to } = await request.json<{ to: string }>();
      const memo = await env.DB.prepare("SELECT body FROM memos WHERE id = ?")
        .bind(share[1])
        .first<{ body: string }>();
      if (!memo) {
        return new Response("not found", { status: 404, headers });
      }
      await sendShareMail(env, to, memo.body);
      return new Response(null, { status: 204, headers });
    }

    return new Response("not found", { status: 404, headers });
  },
};
