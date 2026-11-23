#!/bin/sh
# One price run: restore the last snapshot from the data branch, fetch,
# and force-push the new snapshot as a single commit. Needs BNET_CLIENT_ID,
# BNET_CLIENT_SECRET, GH_TOKEN and GITHUB_REPOSITORY.
set -e
rm -rf out && mkdir out
if git ls-remote --exit-code origin data >/dev/null; then
  git fetch -q --depth 1 origin data
  git --work-tree=out checkout FETCH_HEAD -- .
  git reset -q
fi
python3 job/fetch.py out
cd out
rm -rf .git
git init -q -b data
git config user.name "Kain-Mercer"
git config user.email "339178444+Kain-Mercer@users.noreply.github.com"
git add -A
git commit -q -m "Prices $(date -u +%Y-%m-%dT%H:%MZ)"
git push -q -f "https://x-access-token:${GH_TOKEN}@github.com/${GITHUB_REPOSITORY}" data
cd ..
echo "published $(python3 -c "import json;m=json.load(open('out/meta.json'));print(m['t'], sum(h['items'] for h in m['houses'].values()), 'items')")"
