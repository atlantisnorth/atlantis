module.exports = {
    title: 'Atlantis',
    description: 'Atlantis: Terraform Pull Request Automation',
    ga: "UA-6850151-3",
    head: [
        ['link', { rel: 'icon', type: 'image/png', href: '/favicon-196x196.png', sizes: '196x196' }],
        ['link', { rel: 'icon', type: 'image/png', href: '/favicon-96x96.png', sizes: '96x96' }],
        ['link', { rel: 'icon', type: 'image/png', href: '/favicon-32x32.png', sizes: '32x32' }],
        ['link', { rel: 'icon', type: 'image/png', href: '/favicon-16x16.png', sizes: '16x16' }],
        ['link', { rel: 'icon', type: 'image/png', href: '/favicon-128.png', sizes: '128x128' }],
        ['link', { rel: 'apple-touch-icon-precomposed', sizes: '57x57', href: '/apple-touch-icon-57x57.png' }],
        ['link', { rel: 'apple-touch-icon-precomposed', sizes: '114x114', href: '/apple-touch-icon-114x114.png' }],
        ['link', { rel: 'apple-touch-icon-precomposed', sizes: '72x72', href: '/apple-touch-icon-72x72.png' }],
        ['link', { rel: 'apple-touch-icon-precomposed', sizes: '144x144', href: '/apple-touch-icon-144x144.png' }],
        ['link', { rel: 'apple-touch-icon-precomposed', sizes: '60x60', href: '/apple-touch-icon-60x60.png' }],
        ['link', { rel: 'apple-touch-icon-precomposed', sizes: '120x120', href: '/apple-touch-icon-120x120.png' }],
        ['link', { rel: 'apple-touch-icon-precomposed', sizes: '76x76', href: '/apple-touch-icon-76x76.png' }],
        ['link', { rel: 'apple-touch-icon-precomposed', sizes: '152x152', href: '/apple-touch-icon-152x152.png' }],
        ['meta', {name: 'msapplication-TileColor', content: '#FFFFFF' }],
        ['meta', {name: 'msapplication-TileImage', content: '/mstile-144x144.png' }],
        ['meta', {name: 'msapplication-square70x70logo', content: '/mstile-70x70.png' }],
        ['meta', {name: 'msapplication-square150x150logo', content: '/mstile-150x150.png' }],
        ['meta', {name: 'msapplication-wide310x150logo', content: '/mstile-310x150.png' }],
        ['meta', {name: 'msapplication-square310x310logo', content: '/mstile-310x310.png' }],
        ['link', { rel: 'stylesheet', sizes: '152x152', href: 'https://fonts.googleapis.com/css?family=Lato:400,900' }],
        ['meta', {name: 'google-site-verification', content: 'kTnsDBpHqtTNY8oscYxrQeeiNml2d2z-03Ct9wqeCeE' }]
    ],
    themeConfig: {
        algolia: {
          apiKey: '3b733dff1539ca3a210775860301fa86',
          indexName: 'runatlantis'
        },
        logo: '/hero.png',
        nav: [
            {text: 'Home', link: '/'},
            {text: 'Guide', link: '/guide/'},
            {text: 'Docs', link: '/docs/'},
            {text: 'Blog', link: 'https://medium.com/runatlantis'}
        ],
        sidebar: {
            '/guide/': [
                '',
                'test-drive',
                'testing-locally',
            ],
            '/docs/': [
                {
                    title: 'Installing Atlantis',
                    collapsable: true,
                    children: [
                        'installation-guide',
                        'requirements',
                        'access-credentials',
                        'webhook-secrets',
                        'deployment',
                        'configuring-webhooks',
                        'provider-credentials'
                    ]
                },
                {
                    title: 'Configuring Atlantis',
                    collapsable: true,
                    children: [
                        ['configuring-atlantis', 'Overview'],
                        'server-configuration',
                        'server-side-repo-config',
                        'pre-workflow-hooks',
                        'post-workflow-hooks',
                        'policy-checking',
                        'custom-workflows',
                        'repo-level-atlantis-yaml',
                        'upgrading-atlantis-yaml',
                        'apply-requirements',
                        'checkout-strategy',
                        'terraform-versions',
                        'terraform-cloud'
                    ]
                },
                {
                    title: 'Using Atlantis',
                    collapsable: true,
                    children: [
                        ['using-atlantis', 'Overview']
                    ]
                },
                {
                    title: 'How Atlantis Works',
                    collapsable: true,
                    children: [
                        ['how-atlantis-works', 'Overview'],
                        'locking',
                        'autoplanning',
                        'automerging',
                        'security'
                    ]
                },
                {
                    title: 'Troubleshooting',
                    collapsable: true,
                    children: [
                        'troubleshooting-https',
                    ]
                }
            ]
        },
        repo: 'runatlantis/atlantis',
        docsDir: 'runatlantis.io',
        editLinks: true,
    }
}
