import * as React from 'react'
import { kea, useActions, useValues, path } from 'kea'
import { githubLogic } from './githubLogic'

import type { blaLogicType } from './githubType'

const blaLogic = kea<blaLogicType>([path(['github'])])

function Github() {
    const { username, isLoading, sortedRepositories, error } = useValues(githubLogic)
    const { setUsername } = useActions(githubLogic)

    return (
        <div className="example-github-scene">
            <div style={{ marginBottom: 20 }}>
                <h1>Search for a github user</h1>
                <input value={username} type="text" onChange={(e) => setUsername(e.target.value)} />
            </div>
            {isLoading ? (
                <div>Loading...</div>
            ) : sortedRepositories.length > 0 ? (
                <div>
                    Found {sortedRepositories.length} repositories for user {username}!
                    {sortedRepositories.map((repo) => (
                        <div key={repo.id}>
                            <a href={repo.html_url} target="_blank">
                                {repo.full_name}
                            </a>
                            {' - '}
                            {repo.stargazers_count} stars, {repo.forks} forks.
                        </div>
                    ))}
                </div>
            ) : (
                <div>{error ? `Error: ${error}` : 'No repositories found'}</div>
            )}
        </div>
    )
}
