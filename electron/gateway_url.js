const DEFAULT_GATEWAY_URL = "http://localhost:4567/";

function gatewayUrl(env = process.env, argv = process.argv) {
  const candidate = urlFromArgs(argv) || env.PI_GATEWAY_DESKTOP_URL || DEFAULT_GATEWAY_URL;
  return validHttpUrl(candidate) || DEFAULT_GATEWAY_URL;
}

function urlFromArgs(argv) {
  const prefix = "--gateway-url=";
  const match = argv.find((arg) => arg.startsWith(prefix));
  return match ? match.slice(prefix.length) : null;
}

function validHttpUrl(candidate) {
  try {
    const url = new URL(candidate);
    if (!["http:", "https:"].includes(url.protocol)) return null;
    return url.toString();
  } catch (_error) {
    return null;
  }
}

module.exports = { DEFAULT_GATEWAY_URL, gatewayUrl };
