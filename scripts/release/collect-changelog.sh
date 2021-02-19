#!/bin/bash

ARG_SINCE=
ARG_BRIEF=yes
ARG_CLASSIFY=yes
ARG_GROUP=no
ARG_COMMITS=false
ARG_PRS=true
ARG_REPO=intel/cri-resource-manager

# parse the command line and extract options
parse-commandline () {
    declare -g SCRIPT="${1##*/}"
    shift
    while [ $# -gt 0 ]; do
        case $1 in
            --since|-s)
                ARG_SINCE=$2
                shift
                ;;
            --since=*|-s=*)
                ARG_SINCE="${1##*=}"
                ;;
            --brief|--omit-body|-b)
                ARG_BRIEF=true
                ;;
            --brief=*|--omit-body=*)
                ARG_BRIEF="${1##*=}"
                ;;
            --long|--body)
                ARG_BRIEF=false
                ;;
            --classes)
                ARG_CLASSIFY=true
                ;;
            --no-classes)
                ARG_CLASSIFY=false
                ;;
            --classes=*)
                ARG_CLASSIFY="${1##*=}"
                ;;
            --groups)
                ARG_GROUP=true
                ;;
            --no-groups)
                ARG_GROUP=false
                ;;
            --groups=*)
                ARG_GROUP="${1##*=}"
                ;;
            --repo)
                ARG_REPO=$2
                shift
                ;;
            --repo=*)
                ARG_REPO=${1#*=}
                ;;
            --commits)
                ARG_COMMITS=true
                ;;
            --no-commits)
                ARG_COMMITS=false
                ;;
            --prs)
                ARG_PRS=true
                ;;
            --no-prs)
                ARG_PRS=false
                ;;
            --pr-json)
                ARG_PRJSON=$2
                shift
                ;;
            --pr-json=*)
                ARG_PRJSON=${1#*=}
                shift
                ;;
            --github-token=*|--token=*)
                GITHUB_TOKEN="${1#*=}"
                ;;
            --trace)
                set -x
                ;;
            --help|-h)
                print-usage
                ;;
            *)
                print-usage 1 "unknown command line option \"$1\""
                ;;
        esac
        shift
    done
}

# print help on usage
print-usage () {
    local _ec
    if [ -n "$1" ]; then
        case $1 in
            [0|9]*) _ec=$1; shift;;
        esac
    fi

    cat <<EOF
usage: $SCRIPT [options] --since RELEASE

The possible options are:
    --since RELEASE  generate changelog since RELEASE
    --[no-]commits   whether to collect and analyze commits
    --[no-]prs       whether to collect and analyze PRs
    --brief          include only commit subject in generated output
    --long           include also commit body in generated output
    --[no-]classes   whether to collate commits by subject classification
    --[no-]groups    whether to group commits by subject group (^([^:]*):.*$)
    --[no-]prs       whether to list PRs merged
    --markdown       whether to produce final merged PR list with markdown links
    --token TOKEN    github access tokes used query PRs
    --pr-json FILE   file with list of all PRs in JSON format
    --trace          run with set -x
    --help           produce this help
EOF
    exit ${_ec:-0}
}

# print a progress message to stderr
progress () {
    echo 1>&2 "$@"
}

# produce normal output to stdout
output () {
    echo "$@"
}

# print a fatal error message and exit
fatal () {
    echo 1>&2 "fatal error: " "$@"
    exit 1
}

# setup classification for commits and PRs
setup-classes () {
    declare -g -A class_commits
    declare -g -A class_prs
    declare -g -A class_pattern=(
        [memtier]=policy
        [none]=policy
        [podpools]=policy
        [static]=policy
        [static-plus]=policy
        [static-pools]=policy
        [topology-aware]=policy
        [policy]=policy
        [blockio]=controllers
        [rdt]=controllers
        [page-migrat]=controllers
        [memory]=controllers
        [kata]=controllers
        [cri]=controllers
        [control/]=controllers
        [resmgr]=resource-manager
        [resource-manager]=resource-manager
        [relay]=resource-manager
        [cri/relay]=resource-manager
        [server]=resource-manager
        [cri/server]=resource-manager
        [client]=resource-manager
        [cri/client]=resource-manager
        [cache]=runtime-cache
        [config]=runtime-config
        [cross-build]=build
        [klog]=logging
        [log]=logging
        [cpuallocator]=infra
        [sysfs]=infra
        [utils]=infra
        [test/e2e]=testing
        [e2e]=testing
        [test]=testing
        [demo]=demo
        [Makefile]=build
        [scripts]=scripts
        [cri-resmgr]=cri-resmgr
        [cri-resgrm-agent]=agent
        [cri-resmgr-webook]=webhook
        [doc]=docs
        [docs]=docs
        [README]=docs
        [RELEASE]=docs
        [.github]=repository
        [github]=repository
        [repo]=repository
        [go.mod]=deps
        [go.sum]=deps
    )
    declare -g class_pattern_order=$(echo "${!class_pattern[@]}" | tr -s ' ' '\n' | sort)
}

# gather information about a git commit
git-commit () {
    git log --format=$1 $2^..$2
}

# collect, parse, group and classify commits since $SINCE
collect-commits () {
    local range total_count count sha1 subject groups grp pattern class

    declare -g commit_list
    declare -g -A commit_subject
    declare -g -A commit_groups
    declare -g -A commit_classes
    declare -g -A group_commits
    declare -g -A commit_class
    declare -g -A commits_ungroupped
    declare -g -A commits_unclassified

    if [ -n "$ARG_SINCE" ]; then
        range="$ARG_SINCE..HEAD"
    else
        range="$(git tag -l | tail -1)..HEAD"
    fi

    commit_list=$(git log --no-merges --format=oneline $range | cut -d ' ' -f1)
    total_count=$(echo $commit_list | wc -w)
    count=1
    for sha1 in $commit_list; do
        progress -n "analyze $count/$total_count commit ${sha1%????????????????????????????????}"
        subject="$(git-commit %s $sha1 | tr -s '\t' ' ')"
        subject="${subject%.}"
        commit_subject[$sha1]="$subject"
        case $subject in
            *:*) groups=${subject%%:*}
                 groups=${groups// /}
                 case $groups in
                     *,*) groups="${groups//,/ }";;
                 esac
                 ;;
            *) groups=""
               ;;
        esac
        if [ -n "$groups" ]; then
            progress -n " (groups:"
            for grp in $groups; do
                grp=${grp#pkg/}
                group_commits[$grp]="${group_commits[$grp]} $sha1"
                commit_groups[$sha1]="${commit_groups[$sha1]} $grp"
                progress -n " $grp"
                for pattern in $class_pattern_order; do
                    class="${class_pattern[$pattern]}"
                    case $grp in
                        $pattern)
                            class_commits["$class"]="${class_commits[$class]} $sha1"
                            commit_classes[$sha1]="${commit_groups[$sha1]} $class"
                            commit_classes[$sha1]="${commit_classes[$sha1]} $class"
                            progress -n " (class $class)"
                            break
                            ;;
                        $class) # each class is an implicit pattern
                            class_commits["$class"]="${class_commits[$class]} $sha1"
                            commit_classes[$sha1]="${commit_classes[$sha1]} $class"
                            progress -n " (class $class)"
                            break
                            ;;
                        *)
                            class=""
                            ;;
                    esac
                done
                [ -z "$class" ] && for pattern in $class_pattern_order; do
                    class="${class_pattern[$pattern]}"
                    case $grp in
                        $pattern*)
                            class_commits["$class"]="${class_commits[$class]} $sha1"
                            commit_classes[$sha1]="${commit_classes[$sha1]} $class"
                            progress -n " (class $class)"
                            break
                            ;;
                        $class*) # each class is an implicit pattern
                            class_commits["$class"]="${class_commits[$class]} $sha1"
                            commit_classes[$sha1]="${commit_classes[$sha1]} $class"
                            progress -n " (class $class)"
                            break
                            ;;
                        *)
                            class=""
                            ;;
                    esac
                done
                if [ -z "$class" ]; then
                    commits_unclassified[$sha1]=true
                fi
            done
            progress ")"
        else
            commits_ungroupped[$sha1]=true
            progress " (ungroupped)"
        fi
        let count=$count+1
    done
}

# show the body of a commit indented
show-commit-body () {
    local body sed i=0

    case $ARG_BRIEF in
        true|yes|1) return 0;;
    esac

    sed="s/^/"
    while [ $i -lt ${2:-10} ]; do
        sed="$sed "
        let i=$i+1
    done
    sed="$sed/g"

    git log --format=%b $1^..$1 |
        sed "$sed" | egrep -v '^ *$'
    return 0
}

# sort input or just pass it through
sort-or-cat () {
    case $ARG_BRIEF in
        true|yes|1) sort -u;;
        *)          cat;;
    esac
}

# dump collected commits by classes
dump-commit-classes () {
    case $ARG_CLASSIFY in
        true|yes|1) ;;
        *) return 0;;
    esac

    output "========== Commits Grouped by Class =========="
    for class in $(echo "${!class_commits[@]}" | tr -s ' ' '\n' | sort); do
        output "- $class:"
        (for sha1 in ${class_commits[$class]}; do
            output "    * ${commit_subject[$sha1]}"
            show-commit-body $sha1
         done) | sort-or-cat
        output ""
    done
    if [ "${#commits_unclassified[@]}" -gt 0 ]; then
        output "- unclassified:"
        (for sha1 in ${!commits_unclassified[@]}; do
             if [ -z "${commit_classes[$sha1]}" ]; then
                 output "    * ${commit_subject[$sha1]}"
                 show-commit-body $sha1
             fi
         done) | sort-or-cat
        output ""
    fi
}

# dump collected commits by groups
dump-commit-groups () {
    case $ARG_GROUP in
        true|yes|1) ;;
        *) return 0;;
    esac

    output "========== Commits Grouped by Subject =========="
    for grp in $(echo "${!group_commits[@]}" | tr -s ' ' '\n' | sort); do
        output "- $grp:"
        (for sha1 in ${group_commits[$grp]}; do
            output "    * ${commit_subject[$sha1]}"
            show-commit-body $sha1
        done) | sort-or-cat
        output ""
    done
    if [ "${#commits_ungroupped[@]}" -gt 0 ]; then
        output "- ungroupped:"
        (for sha1 in ${!commits_ungroupped[@]}; do
             output "    * ${commit_subject[$sha1]}"
             show-commit-body $sha1
         done) | sort-or-cat
        output ""
    fi
}

# fetch all PRs as JSON
fetch-pr-json () {
    declare -g pr_json

    if [ -f "$ARG_PRJSON" ]; then
        pr_json="$ARG_PRJSON"
        return 0
    fi

    if [ -z "$GITHUB_TOKEN" ]; then
        if [ -f ~/.github/access-token ]; then
            GITHUB_TOKEN=$(cat ~/.github/access-token)
        else
            print-usage 1 "no GITHUB_TOKEN set"
        fi
    fi

    pr_json=$(mktemp)
    progress "fetching github PR list and metadata ($pr_json)..."
    gh api "repos/$ARG_REPO/pulls?state=all&per_page=100" --paginate > $pr_json ||
        fatal "failed to fetch list of PRs"
}

# select PRs of interest for us
select-prs () {
    local range pr query or beg end result

    beg=$(echo $pr_list | tr -s ' ' '\n' | egrep '^[0-9]*' | head -1)
    end=$(echo $pr_list | tr -s ' ' '\n' | egrep '^[0-9]*' | tail -1)
    result="$pr_json.${beg}-${end}"
    if [ ! -e "$result" ]; then
        for pr in $pr_list; do
            query="${query}${or}.number == $pr "
            or=" or "
        done
        jq -c ".[] | select ( $query ) | {number:.number,title:.title,body:.body}" $pr_json \
           > $result
    fi
    echo $result
}

# collect PRs
collect-prs () {
    case $ARG_PRS in
        true|yes|1) ;;
        *) return 0;;
    esac

    local range pr_range pr total_count count sha1 subject groups grp pattern class
    declare -g pr_list
    declare -g -A pr_subject
    declare -g -A pr_groups
    declare -g -A pr_classes
    declare -g -A group_prs
    declare -g -A pr_class
    declare -g -A prs_ungroupped
    declare -g -A prs_unclassified

    if [ -n "$ARG_SINCE" ]; then
        range="$ARG_SINCE..HEAD"
    else
        range="$(git tag -l | tail -1)..HEAD"
    fi

    pr_list=$(git log --merges --format=oneline $range |
                  grep 'Merge pull request #' | cut -d ' ' -f 5 | tr -d '#' | sort -n)

    fetch-pr-json
    pr_range=$(select-prs)
        
    total_count=$(echo $pr_list | wc -w)
    count=1
    for pr in $pr_list; do
        progress -n "analyze $count/$total_count PR #$pr"
        subject=$(jq -c "select ( .number == $pr ) | .title" $pr_range)
        subject="${subject#\"}"; subject="${subject%\"}"; subject="${subject%.}"
        pr_subject[$pr]="$subject"
        case $subject in
            *:*) groups=${subject%%:*}
                 groups=${groups// /}
                 case $groups in
                     *,*) groups="${groups//,/ }";;
                 esac
                 ;;
            *) groups=""
               ;;
        esac
        if [ -n "$groups" ]; then
            progress -n " (groups:"
            for grp in $groups; do
                grp=${grp#pkg/}
                group_prs[$grp]="${group_prs[$grp]} $pr"
                pr_groups[$pr]="${pr_groups[$pr]} $grp"
                progress -n " $grp"
                for pattern in $class_pattern_order; do
                    class="${class_pattern[$pattern]}"
                    case $grp in
                        $pattern)
                            class_prs["$class"]="${class_prs[$class]} $pr"
                            pr_classes[$pr]="${pr_groups[$pr]} $class"
                            pr_classes[$pr]="${pr_classes[$pr]} $class"
                            progress -n " (class $class)"
                            break
                            ;;
                        $class) # each class is an implicit pattern
                            class_prs["$class"]="${class_prs[$class]} $pr"
                            pr_classes[$pr]="${pr_classes[$pr]} $class"
                            progress -n " (class $class)"
                            break
                            ;;
                        *)
                            class=""
                            ;;
                    esac
                done
                [ -z "$class" ] && for pattern in $class_pattern_order; do
                    class="${class_pattern[$pattern]}"
                    case $grp in
                        $pattern*)
                            class_prs["$class"]="${class_prs[$class]} $pr"
                            pr_classes[$pr]="${pr_classes[$pr]} $class"
                            progress -n " (class $class)"
                            break
                            ;;
                        $class*) # each class is an implicit pattern
                            class_prs["$class"]="${class_prs[$class]} $pr"
                            pr_classes[$pr]="${pr_classes[$pr]} $class"
                            progress -n " (class $class)"
                            break
                            ;;
                        *)
                            class=""
                            ;;
                    esac
                done
                if [ -z "$class" ]; then
                    prs_unclassified[$pr]=true
                fi
            done
            progress ")"
        else
            prs_ungroupped[$pr]=true
            progress " (ungroupped)"
        fi
        let count=$count+1
    done

}

# dump collected PRs by classes
dump-pr-classes () {
    case $ARG_CLASSIFY in
        true|yes|1) ;;
        *) return 0;;
    esac

    local class pr

    output "========== PRs Grouped by Class =========="
    for class in $(echo "${!class_prs[@]}" | tr -s ' ' '\n' | sort); do
        output "- $class:"
        (for pr in ${class_prs[$class]}; do
            output "    * ${pr_subject[$pr]} (PR #$pr)"
            show-pr-body $pr
         done) | sort-or-cat
        output ""
    done
    if [ "${#prs_unclassified[@]}" -gt 0 ]; then
        output "- unclassified:"
        (for pr in ${!prs_unclassified[@]}; do
             if [ -z "${pr_classes[$pr]}" ]; then
                 output "    * ${pr_subject[$pr]} (PR #$pr)"
                 show-pr-body $pr
             fi
         done) | sort-or-cat
        output ""
    fi
}

# dump collected commits by groups
dump-pr-groups () {
    case $ARG_GROUP in
        true|yes|1) ;;
        *) return 0;;
    esac

    local grp pr

    output "========== PRs Grouped by Subject =========="
    for grp in $(echo "${!group_prs[@]}" | tr -s ' ' '\n' | sort); do
        output "- $grp:"
        (for pr in ${group_prs[$grp]}; do
            output "    * ${pr_subject[$pr]}"
            show-pr-body $pr
        done) | sort-or-cat
        output ""
    done
    if [ "${#prs_ungroupped[@]}" -gt 0 ]; then
        output "- ungroupped:"
        (for pr in ${!prs_ungroupped[@]}; do
             output "    * ${pr_subject[$pr]}"
             show-pr-body $pr
         done) | sort-or-cat
        output ""
    fi
}

# show the body of a PR indented
show-pr-body () {
    local body sed i=0

    return 0

    case $ARG_BRIEF in
        true|yes|1) return 0;;
    esac

    sed="s/^/"
    while [ $i -lt ${2:-10} ]; do
        sed="$sed "
        let i=$i+1
    done
    sed="$sed/g"

    git log --format=%b $1^..$1 |
        sed "$sed" | egrep -v '^ *$'
    return 0
}

# dump the list of merged PRs.
dump-pr-list () {
    local pr subject
    output "List of merged PRs:"
    for pr in $pr_list; do
        subject="${pr_subject[$pr]}"
        echo "  - PR #$pr: $subject"
    done
}

# perform and dump the result of commit analysis
analyze-commits () {
    case $ARG_COMMITS in
        true|yes|1) ;;
        *) return 0;;
    esac
    collect-commits
    dump-commit-classes
    dump-commit-groups
}

# perform and dump the result of PR analysis
analyze-prs () {
    case $ARG_PRS in
        true|yes|1) ;;
        *) return 0;;
    esac
    collect-prs
    dump-pr-classes
    dump-pr-groups
    dump-pr-list
}

# main script
main () {
    setup-classes
    analyze-commits
    analyze-prs
}

parse-commandline $0 "$@"
main

