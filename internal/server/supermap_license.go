package server

import "net/http"

const superMapLicensePath = "/iserver/manager/license.json"

var superMapLicenseJSON = []byte(`{
  "iServerStandard": true,
  "iServerSpatialStreaming": true,
  "iServerPlot": true,
  "mServerBattlefield": true,
  "iServerSpatial": true,
  "mServerBasic": true,
  "iServerProfessional": true,
  "iServerSituationEvolution": true,
  "iServerVideoService": true,
  "iServerChart": true,
  "mServerProfessional": true,
  "iServerDataBaseEngine": true,
  "iServerSpaceBasic": true,
  "iServerNetwork": true,
  "builder": {},
  "iServerEnterprise": true,
  "remoteSensingLicenseTypeStruct": {
    "iServerPIM_Building": true,
    "iServerPIM_Greenhouse": true,
    "iServerPIM_Cloud": true,
    "iServerPIM_Farmland": true,
    "iServerPIM_Woodland": true,
    "iServerLIM_LULC": true,
    "iServerPIM_Water": true
  },
  "productType": "iServer",
  "iServerSpatialProcessing": true,
  "iServerThreeddesigner": true,
  "iServerTrafficTransfer": true,
  "mServerMilitaryAnalyst": true,
  "iServerKnowledgeService": true,
  "mServerOnBoard10": true,
  "iServerBasicSpatialAnalysis": true,
  "iServerBasic": true,
  "mServerEnterprise": true,
  "mServerOnBoard3": true,
  "iServerServiceNodeAddition": true,
  "mServerDynamicTarget": true,
  "trialVersion": true,
  "iServerGeoBlockchain": true,
  "mServerUltra": true,
  "iServerImage": true,
  "mServerStandard": true,
  "iServerSpace": true,
  "iServerUltra": true
}
`)

func (s *Server) writeSuperMapLicense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		s.writeSuperMapError(w, r, http.StatusMethodNotAllowed, "only GET, HEAD and OPTIONS are supported")
		return
	}
	writeResponse(w, r, http.StatusOK, "application/json; charset=utf-8", superMapLicenseJSON)
}
