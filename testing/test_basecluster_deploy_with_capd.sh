
set -e

repourl=http://172.17.0.1:8188/myrepo.git
repodir=/tmp/arlon-testbed-git-clone/myrepo
repopath=baseclusters/capd-test
baseclusterdir=${repodir}/${repopath}
manifestfile=capd-capi-quickstart-withclusternamelabelremoved.yaml

if ! [ -f ${baseclusterdir}/capd-capi-quickstart-withclusternamelabelremoved.yaml ]; then
    echo adding basecluster directory
    mkdir -p ${baseclusterdir}
    cp testing/${manifestfile} ${baseclusterdir}
    pushd ${baseclusterdir}
    git pull
    git add ${manifestfile}
    git commit -m ${manifestfile}
    git push origin main
    popd
fi

if arlon basecluster validategit --repo-url ${repourl} --repo-path ${repopath} 2> /tmp/stdout.txt; then
    echo ${repopath} already prepped
else
    if ! grep "Error: kustomization.yaml is missing" /tmp/stdout.txt &> /dev/null ; then
        echo unexpected output from validategit, check /tmp/stdout.txt
        exit 1
    fi
    if ! arlon basecluster preparegit --repo-url ${repourl} --repo-path ${repopath}; then
        echo preparegit failed
        exit 1
    fi
fi
