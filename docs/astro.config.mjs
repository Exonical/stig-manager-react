// @ts-check
import { defineConfig } from 'astro/config'
import starlight from '@astrojs/starlight'
import starlightOpenAPI, { openAPISidebarGroups } from 'starlight-openapi'

// https://astro.build/config
export default defineConfig({
  site: 'https://stig-manager-react.exonical.dev',
  trailingSlash: 'never',
  integrations: [
    starlight({
      title: 'STIG Manager',
      description:
        'Open-source API and web client for managing STIG assessments. ' +
        'React 19 + shadcn/ui frontend, Go backend, Postgres 18.',
      logo: { src: './src/assets/logo.svg', replacesTitle: false },
      favicon: '/favicon.svg',
      defaultLocale: 'en',
      lastUpdated: true,
      pagination: true,
      tableOfContents: { minHeadingLevel: 2, maxHeadingLevel: 4 },
      editLink: {
        baseUrl:
          'https://github.com/Exonical/stig-manager-react/edit/main/docs/',
      },
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/Exonical/stig-manager-react',
        },
      ],
      customCss: ['./src/styles/custom.css'],
      plugins: [
        starlightOpenAPI([
          {
            base: 'reference/api',
            label: 'STIG Manager API',
            schema: './openapi/stig-manager.yaml',
          },
        ]),
      ],
      sidebar: [
        { label: 'Introduction', slug: 'index' },
        {
          label: 'Features',
          collapsed: false,
          items: [
            { label: 'Overview', slug: 'features/overview' },
            { label: 'Common Tasks', slug: 'features/common-tasks' },
          ],
        },
        {
          label: 'Installation & Setup',
          collapsed: false,
          items: [
            { label: 'Overview', slug: 'installation/overview' },
            { label: 'Quick Start (Docker)', slug: 'installation/quickstart' },
            { label: 'Database (Postgres 18)', slug: 'installation/database' },
            { label: 'Authentication (OIDC)', slug: 'installation/authentication' },
            { label: 'Environment Variables', slug: 'installation/environment-variables' },
            { label: 'Data & Permissions', slug: 'installation/data-and-permissions' },
            { label: 'Logging', slug: 'installation/logging' },
            { label: 'Reverse Proxy', slug: 'installation/reverse-proxy' },
            { label: 'Hardening & TLS', slug: 'installation/securing' },
          ],
        },
        {
          label: 'User Guide',
          collapsed: false,
          items: [
            { label: 'Quick Start', slug: 'user-guide/quickstart' },
            { label: 'Concepts & Workflow', slug: 'user-guide/concepts' },
            { label: 'Roles & Access', slug: 'user-guide/roles-and-access' },
            { label: 'Review Handling', slug: 'user-guide/review-handling' },
            { label: 'Rule Exceptions', slug: 'user-guide/rule-exceptions' },
          ],
        },
        {
          label: 'Admin Guide',
          collapsed: false,
          items: [
            { label: 'Quick Start', slug: 'admin-guide/quickstart' },
            { label: 'Operations', slug: 'admin-guide/operations' },
          ],
        },
        {
          label: 'Reference',
          collapsed: false,
          items: [
            { label: 'Overview', slug: 'reference/overview' },
            ...openAPISidebarGroups,
          ],
        },
        {
          label: 'The Project',
          collapsed: true,
          items: [
            { label: 'Project Description', slug: 'project/description' },
            { label: 'Architecture', slug: 'project/architecture' },
            { label: 'Contributing', slug: 'project/contributing' },
            { label: 'Testing', slug: 'project/testing' },
            { label: 'Related Repos', slug: 'project/related-repos' },
            { label: 'Roadmap', slug: 'project/roadmap' },
            { label: 'Examples', slug: 'project/examples' },
            { label: 'License & Intent', slug: 'project/license' },
          ],
        },
      ],
    }),
  ],
})
