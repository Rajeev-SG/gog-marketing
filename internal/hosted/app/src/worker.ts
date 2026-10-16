/** Production entrypoint: keep the native Durable Object export out of Node-only route tests. */
export { default } from "./index.js";
export { Chat } from "./chat/agent.js";
