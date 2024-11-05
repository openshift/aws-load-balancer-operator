# Versioning and Branching in AWS Load Balancer Operator

The AWS Load Balancer Operator follows the semantic versioning, for any given release `X.Y.Z`:
* an X (major) release indicates a set of backwards-compatible changes. Changing X means there's a breaking change.
* a Y (minor) release indicates a minimum feature set. Changing Y means the addition of a backwards-compatible feature.
* a Z (patch) release indicates minimum set of bugfixes. Changing Z means a backwards-compatible change that doesn't add functionality.

## Branches

The AWS Load Balancer Operator repository contains two types of branches: the `main` branch and `release-X.Y` branches.

The main branch is where development happens. All the latest code, including breaking changes, happens on `main`.
The `release-X.Y` branches contain stable, backwards compatible code. Every minor (`X.Y`) release, a new such branch is created.

## Channels

The AWS Load Balancer Operator's releases get published in two types of [OLM channels](https://olm.operatorframework.io/docs/glossary/#channel): the minor `release-vX.Y` and the major `release-vX`.

The minor channels contain patch releases. The major channels contain all patch releases from all minor channels.

## OpenShift Version Compatibility

The table below illustrates the OpenShift versions for which various AWS Load Balancer Operator releases were intended:

| OCP version | AWS Load Balancer Operator branch  | AWS Load Balancer Operator OLM channel |
| :---------: | :-------------------------------:  | :------------------------------------: |
| 4.16        | release-1.1                        | stable-v1.1, stable-v1                 |
| 4.15        | release-1.1                        | stable-v1.1, stable-v1                 |
| 4.14        | release-1.1                        | stable-v1.1, stable-v1                 |
| 4.13        | release-1.0                        | stable-v1.0, stable-v1                 |
| 4.12        | release-0.2                        | stable-v0.2, stable-v0                 |
| 4.11        | release-0.1                        | stable-v0.1, stable-v0                 |

## Support model

| AWS Load Balancer Operator release | Support model   |
| :--------------------------------: | :-------------: |
| 1.1                                | Full Support    |
| 1.0                                | Full Support    |
| 0.2                                | End of Life     |
| 0.1                                | End of Life     |

### Full support

During the Full Support phase, qualified critical and important security fixes will be released as they become available.
Urgent and high priority bug fixes will be released as they become available. Other fixes and qualified patches may be released via periodic updates.
To receive security and bug fixes, users are expected to upgrade the operator to the most current supported patch version.
Users should install operator releases on the designated OpenShift versions (see [OpenShift Version Compatibility](#openshift-version-compatibility)).
