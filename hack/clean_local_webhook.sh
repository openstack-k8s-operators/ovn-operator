#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration/vovndbcluster.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/movndbcluster.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vovnnorthd.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/movnnorthd.kb.io --ignore-not-found
oc delete validatingwebhookconfiguration/vovncontroller.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration/movncontroller.kb.io --ignore-not-found
