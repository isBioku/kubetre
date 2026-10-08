// Modified for KubeTRE: configuration is supplied at runtime by /config.js
// (window.__TRE_CONFIG__) instead of a config.json baked in at build time, so one image
// serves every environment. Defaults are AzureTRE's config.source.json.
import defaults from "./config.source.json";

const runtime = (window as any).__TRE_CONFIG__ || {};
const config = { ...defaults, ...runtime };

export default config;
