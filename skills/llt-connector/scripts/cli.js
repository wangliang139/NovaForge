#!/usr/bin/env node

const fs = require('node:fs');
const process = require('node:process');

function fail(message) {
  console.error(message);
  process.exit(1);
}

function readEnv(name) {
  const value = (process.env[name] || '').trim();
  if (!value) {
    fail(`missing required environment variable: ${name}`);
  }
  return value;
}

function readOptionalEnv(name, fallback = '') {
  const value = (process.env[name] || '').trim();
  return value || fallback;
}

function readStdin() {
  return new Promise((resolve, reject) => {
    let data = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (chunk) => {
      data += chunk;
    });
    process.stdin.on('end', () => resolve(data));
    process.stdin.on('error', reject);
  });
}

async function readTextArg(value, filePath, label) {
  if (value) {
    return value;
  }
  if (filePath) {
    return fs.readFileSync(filePath, 'utf8');
  }
  if (!process.stdin.isTTY) {
    const stdin = await readStdin();
    if (stdin.trim()) {
      return stdin;
    }
  }
  fail(`provide ${label} with --query, --query-file, or stdin`);
}

function readJsonArg(value, filePath) {
  if (value && filePath) {
    fail('use either inline JSON or a JSON file, not both');
  }
  if (filePath) {
    return JSON.parse(fs.readFileSync(filePath, 'utf8'));
  }
  if (value) {
    return JSON.parse(value);
  }
  return {};
}

function buildUrl(baseUrl, envKey, defaultPath) {
  const override = (process.env[envKey] || '').trim();
  if (override) {
    return override;
  }
  return new URL(defaultPath, `${baseUrl.replace(/\/+$/, '')}/`).toString();
}

function parseArgs(argv) {
  const result = {
    positionals: [],
    flags: {},
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg.startsWith('--')) {
      result.positionals.push(arg);
      continue;
    }

    const key = arg.slice(2);
    const next = argv[i + 1];
    if (!next || next.startsWith('--')) {
      result.flags[key] = true;
      continue;
    }
    result.flags[key] = next;
    i += 1;
  }

  return result;
}

function printHelp() {
  console.log(`usage: node scripts/cli.js graphql [options]

options:
  --query <string>            inline GraphQL query
  --query-file <path>         read GraphQL query from file
  --variables <json>          inline JSON variables
  --variables-file <path>     read JSON variables from file
  --operation-name <string>   GraphQL operation name
  --timeout-ms <number>       request timeout in milliseconds

environment:
  LLT_BASE_URL                base gateway URL, default is https://localhost:3000
  LLT_API_KEY                 API key used as X-API-KEY
`);
}

async function runGraphql(flags) {
  if (typeof fetch !== 'function') {
    fail('global fetch is unavailable; use Node.js 18+');
  }

  const baseUrl = readOptionalEnv('LLT_BASE_URL', 'https://localhost:3000');
  const apiKey = readEnv('LLT_API_KEY');
  const queryFile = typeof flags['query-file'] === 'string' ? flags['query-file'] : null;
  const variablesFile = typeof flags['variables-file'] === 'string' ? flags['variables-file'] : null;
  const query = await readTextArg(flags.query, queryFile, 'a GraphQL query');
  const variables = readJsonArg(flags.variables, variablesFile);
  const operationName = typeof flags['operation-name'] === 'string' ? flags['operation-name'] : undefined;
  const timeoutMsRaw = typeof flags['timeout-ms'] === 'string' ? Number.parseInt(flags['timeout-ms'], 10) : 30000;
  if (!Number.isFinite(timeoutMsRaw) || timeoutMsRaw <= 0) {
    fail('timeout must be a positive integer in milliseconds');
  }
  const timeoutMs = timeoutMsRaw;
  const url = buildUrl(baseUrl, 'LLT_QUERY_URL', '/query');

  const payload = {
    query,
    variables,
  };
  if (operationName) {
    payload.operationName = operationName;
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

  let response;
  try {
    response = await fetch(url, {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        'X-API-KEY': apiKey,
      },
      body: JSON.stringify(payload),
      signal: controller.signal,
    });
  } catch (error) {
    clearTimeout(timeoutId);
    fail(`request failed: ${error instanceof Error ? error.message : String(error)}`);
  }
  clearTimeout(timeoutId);

  const rawText = await response.text();
  let body = {};
  if (rawText.trim()) {
    try {
      body = JSON.parse(rawText);
    } catch (error) {
      body = { raw: rawText };
    }
  }

  console.log(JSON.stringify(body, null, 2));

  if (!response.ok) {
    process.exit(1);
  }
  if (body && typeof body === 'object' && Array.isArray(body.errors) && body.errors.length > 0) {
    process.exit(1);
  }
}

async function main() {
  const { positionals, flags } = parseArgs(process.argv.slice(2));

  if (flags.help || flags.h) {
    printHelp();
    return;
  }

  const command = positionals[0];
  if (!command) {
    printHelp();
    process.exit(1);
  }
  if (command !== 'graphql') {
    fail(`unsupported command: ${command}`);
  }

  await runGraphql(flags);
}

main().catch((error) => {
  fail(error instanceof Error ? error.message : String(error));
});
