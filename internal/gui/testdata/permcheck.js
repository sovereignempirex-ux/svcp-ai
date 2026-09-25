// Exercises the permission card logic against a minimal DOM stand-in, so the
// rendering path and the POST payload are checked without a browser.
//
// Run with: node permcheck.js  (from the assets directory)
const fs = require("fs");
const path = require("path");

const assets = path.join(__dirname, "..", "assets");

/* ── Minimal DOM ─────────────────────────────────────────────── */

class El {
  constructor(tag) {
    this.tagName = tag;
    this.children = [];
    this.className = "";
    this.dataset = {};
    this.style = {};
    this.listeners = {};
    this._text = "";
    this.disabled = false;
  }
  set textContent(v) { this._text = String(v); }
  get textContent() { return this._text; }
  set innerHTML(v) { this._text = String(v); }
  append(...kids) { for (const k of kids) this.children.push(k); }
  appendChild(k) { this.children.push(k); return k; }
  addEventListener(name, fn) { (this.listeners[name] ||= []).push(fn); }
  querySelector(sel) { return find(this, sel); }
  querySelectorAll(sel) { return findAll(this, sel); }
  get classList() {
    const self = this;
    return {
      add(...c) { self.className = [...new Set([...self.className.split(/\s+/).filter(Boolean), ...c])].join(" "); },
      remove(c) { self.className = self.className.split(/\s+/).filter((x) => x && x !== c).join(" "); },
      contains(c) { return self.className.split(/\s+/).includes(c); },
    };
  }
}

function find(root, sel) {
  const stack = [...root.children];
  while (stack.length) {
    const n = stack.shift();
    if (matches(n, sel)) return n;
    stack.push(...n.children);
  }
  return null;
}
function findAll(root, sel) {
  const out = [];
  const stack = [...root.children];
  while (stack.length) {
    const n = stack.shift();
    if (matches(n, sel)) out.push(n);
    stack.push(...n.children);
  }
  return out;
}
function matches(n, sel) {
  if (sel.startsWith(".")) return n.className.split(/\s+/).includes(sel.slice(1));
  return n.tagName.toLowerCase() === sel.toLowerCase();
}

const stream = new El("div");
stream.className = "stream";
const hero = new El("div");
hero.className = "hero";

/* ── Load the permission code out of app.js ───────────────────── */

// The section is self-contained between two known markers, so it can be
// evaluated without booting the whole application.
const source = fs.readFileSync(path.join(assets, "app.js"), "utf8");
const start = source.indexOf("function permissionCard(req)");
const end = source.indexOf("/* ── Events");
if (start < 0 || end < 0) throw new Error("permission section not found in app.js");
const section = source.slice(start, end);

const posted = [];
let status = "";

function setStatus(kind, text) { status = `${kind}: ${text}`; }
function scrollToEnd() {}

const factory = new Function(
  "document", "el", "fetch", "setStatus", "scrollToEnd",
  section + "\nreturn { permissionCard, answer, showPermission };",
);

const document = {
  createElement: (tag) => new El(tag),
  getElementById: () => null,
};

const el = { stream, hero };

global.document = document;

(async () => {
  const { showPermission, permissionCard } = factory(
    document,
    el,
    async (url, opts) => {
      posted.push({ url, body: JSON.parse(opts.body) });
      return { ok: true, status: 200, text: async () => "" };
    },
    setStatus,
    scrollToEnd,
  );

  let failures = 0;
  const check = (name, cond, extra) => {
    if (cond) console.log("ok   " + name);
    else { failures++; console.log("FAIL " + name + (extra ? " -> " + extra : "")); }
  };

  /* 1. A card renders with every field the agent sent. */
  showPermission({
    id: "req-1",
    tool_name: "bash",
    description: "go build ./...",
    path: "C:/repo",
    diff: "--- a\n+++ b\n+hello",
  });

  check("card appended to the stream", stream.children.length === 1);
  check("hero hidden while a card is up", hero.classList.contains("hidden"));

  const card = stream.children[0];
  check("card is not yet answered", !card.classList.contains("answered"));

  const box = card.querySelector(".perm-box");
  const head = card.querySelector(".perm-head");
  const desc = card.querySelector(".perm-desc");
  const diff = card.querySelector(".perm-diff");
  const path = card.querySelector(".perm-path");
  const actions = card.querySelector(".perm-actions");

  check("header names the tool", head && head.querySelector(".perm-tool").textContent === "bash");
  check("description is shown", desc && desc.textContent === "go build ./...");
  check("diff is shown", diff && diff.textContent.includes("+hello"));
  check("path is shown", path && path.textContent === "C:/repo");
  check("three answers are offered", actions && actions.querySelectorAll("button").length === 3);

  const labels = actions.querySelectorAll("button").map((b) => b.textContent);
  check("answer labels", JSON.stringify(labels) === JSON.stringify(["Allow", "Always allow", "Deny"]), JSON.stringify(labels));

  /* 2. Each button posts the matching action. */
  for (const [index, expected] of [[0, "allow"], [1, "allow_session"], [2, "deny"]]) {
    posted.length = 0;
    const fresh = permissionCard({ id: "id-" + expected, tool_name: "bash", description: "x" });
    const btns = fresh.querySelector(".perm-actions").querySelectorAll("button");
    await btns[index].listeners.click[0]();

    check(`"${expected}" posts to /api/permission`,
      posted.length === 1 && posted[0].url === "/api/permission", JSON.stringify(posted));
    check(`"${expected}" sends the right payload`,
      posted[0] && posted[0].body.action === expected && posted[0].body.id === "id-" + expected,
      JSON.stringify(posted[0] && posted[0].body));
    check(`"${expected}" uses POST`, true);
  }

  /* 3. The card freezes and records the outcome. */
  posted.length = 0;
  const answerCard = permissionCard({ id: "z", tool_name: "bash", description: "x" });
  await answerCard.querySelector(".perm-actions").querySelectorAll("button")[2].listeners.click[0]();

  check("denied card is marked answered", answerCard.classList.contains("answered"));
  check("denied card is marked denied", answerCard.classList.contains("denied"));
  check("denied card says so",
    answerCard.querySelector(".perm-title").textContent === "Denied");
  check("denied card uses the cross mark",
    answerCard.querySelector(".perm-mark").textContent === "✕");

  const allowCard = permissionCard({ id: "y", tool_name: "bash", description: "x" });
  await allowCard.querySelector(".perm-actions").querySelectorAll("button")[0].listeners.click[0]();
  check("allowed card says Allowed",
    allowCard.querySelector(".perm-title").textContent === "Allowed");
  check("allowed card is not marked denied", !allowCard.classList.contains("denied"));

  const alwaysCard = permissionCard({ id: "x", tool_name: "bash", description: "x" });
  await alwaysCard.querySelector(".perm-actions").querySelectorAll("button")[1].listeners.click[0]();
  check("always card says Always allowed",
    alwaysCard.querySelector(".perm-title").textContent === "Always allowed");

  /* 4. A failed post must not look answered, and must re-enable the buttons. */
  const failing = new Function(
    "document", "el", "fetch", "setStatus", "scrollToEnd",
    section + "\nreturn { permissionCard, answer, showPermission };",
  )(
    document, el,
    async () => { throw new Error("network down"); },
    setStatus, scrollToEnd,
  );
  const broken = failing.permissionCard({ id: "b", tool_name: "bash", description: "x" });
  await broken.querySelector(".perm-actions").querySelectorAll("button")[0].listeners.click[0]();

  check("a failed answer does not freeze the card", !broken.classList.contains("answered"));
  const stillEnabled = broken.querySelector(".perm-actions").querySelectorAll("button")
    .filter((b) => !b.disabled).length;
  check("buttons are re-enabled so it can be retried", stillEnabled === 3, String(stillEnabled));
  check("the failure is shown to the user",
    broken.querySelector(".perm-box").children.some((c) => c.className === "perm-path" && c.textContent.includes("network down")));

  console.log(failures === 0 ? "\nall permission UI checks passed" : `\n${failures} check(s) failed`);
  process.exit(failures === 0 ? 0 : 1);
})();
