/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
type Lang = 'curl' | 'python' | 'typescript' | 'javascript'
type SampleContext = {
  baseUrl: string
  apiKeyEnv: string
  modelName: string
  endpointType: string
  endpointPath: string
}

export function buildDecisionsSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const body =
    ctx.endpointType === 'openai-decisions'
      ? {
          model: ctx.modelName,
          input: 'The sky is blue.',
          questions: [
            {
              type: 'predicate',
              instructions: 'Is the sky described as blue?',
            },
          ],
        }
      : {
          model: ctx.modelName,
          state: 'The sky is blue.',
          questions: {
            blue: {
              type: 'noul',
              instructions: 'Is the sky described as blue?',
            },
          },
        }
  const json = JSON.stringify(body, null, 2)
  if (lang === 'curl')
    {return `curl ${JSON.stringify(url)} \\\n  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\\n  -H "Content-Type: application/json" \\\n  -d '${json}'`}
  if (lang === 'python')
    {return `import json\nimport os\nfrom urllib.request import Request, urlopen\n\nbody = json.loads(${JSON.stringify(json)})\nrequest = Request(\n    ${JSON.stringify(url)},\n    data=json.dumps(body).encode(),\n    headers={\n        "Authorization": f"Bearer {os.environ['${ctx.apiKeyEnv}']}",\n        "Content-Type": "application/json",\n    },\n)\nwith urlopen(request) as response:\n    print(json.load(response)["answers"])`}
  return `const response = await fetch(${JSON.stringify(url)}, {\n  method: 'POST',\n  headers: {\n    Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\`,\n    'Content-Type': 'application/json',\n  },\n  body: JSON.stringify(${json}),\n})\nif (!response.ok) throw new Error(\`HTTP \${response.status}\`)\nconsole.log((await response.json()).answers)`
}
