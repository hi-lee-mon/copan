import "@typespec/http";
using Http;

@service(#{ title: "Copan API" })
namespace Copan;

@route("/health")
@get
op check(): HealthResponse;

model HealthResponse {
  version?: string;
  status:"OK";
}
