import { listMemos, saveMemo, shareMemo } from "./api";

const list = document.querySelector<HTMLUListElement>("#memos")!;
const form = document.querySelector<HTMLFormElement>("#new-memo")!;

async function render(): Promise<void> {
  const memos = await listMemos();
  list.replaceChildren(
    ...memos.map((memo) => {
      const item = document.createElement("li");
      item.textContent = memo.body;
      const share = document.createElement("button");
      share.textContent = "共有";
      share.onclick = async () => {
        const to = prompt("共有先のメールアドレス");
        if (to) {
          await shareMemo(memo.id, to);
        }
      };
      item.append(share);
      return item;
    }),
  );
}

form.onsubmit = async (event) => {
  event.preventDefault();
  const body = new FormData(form).get("body");
  if (typeof body === "string" && body.trim() !== "") {
    await saveMemo(body);
    form.reset();
    await render();
  }
};

render();
