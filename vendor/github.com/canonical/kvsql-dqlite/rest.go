package factory

import (
	restful "github.com/emicklei/go-restful"
)

type Rest struct{}

func (r Rest) Install(c *restful.Container) {
	ws := new(restful.WebService)
	ws.Path("/dqlite").Consumes(restful.MIME_JSON).Produces(restful.MIME_JSON)
	ws.Doc("dqlite cluster management")
	ws.Route(ws.GET("/").To(getHandler))
	c.Add(ws)
}

func getHandler(req *restful.Request, resp *restful.Response) {
	foo := struct {
		A string
		B int
	}{
		A: "foo",
		B: 123,
	}
	resp.WriteEntity(foo)
}
