// The kejilion.sh docker_app_plus contract shared by the onboarding guide and
// the local app editor. app_id must equal the file name for install markers.
export function localAppTemplate(name = 'myapp'): string {
  return `local app_id="${name}"
local app_name="My App"
local app_text="A brief description"
local app_url="https://github.com/..."
local docker_name="${name}"
local docker_port="8080"
local app_size="1"

docker_app_install() {
\tmkdir -p /home/docker/${name} && cd /home/docker/${name}
\tcurl -L -o docker-compose.yml "\${gh_proxy}https://raw.githubusercontent.com/..."
\tsed -i "s/8080:8080/\${docker_port}:8080/g" docker-compose.yml
\tdocker compose up -d
\tcheck_docker_app_ip
}

docker_app_update() {
\tcd /home/docker/${name} && docker compose pull && docker compose up -d
}

docker_app_uninstall() {
\tcd /home/docker/${name} && docker compose down --rmi all
\trm -rf /home/docker/${name}
}

docker_app_plus
`
}

export const localAppNamePattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/

const officialRepository = 'https://github.com/kejilion/apps'
// GitHub truncates very long prefilled URLs; past this the editor copies instead.
const maxPrefilledUrlLength = 7000

/** Opens GitHub's new-file page with the config prefilled; GitHub forks and opens a PR for non-members. */
export function officialSubmissionUrl(name: string, content: string): { url: string; prefilled: boolean } {
  const base = `${officialRepository}/new/main?filename=${encodeURIComponent(`${name}.conf`)}`
  const prefilled = `${base}&value=${encodeURIComponent(content)}`
  return prefilled.length <= maxPrefilledUrlLength ? { url: prefilled, prefilled: true } : { url: base, prefilled: false }
}
