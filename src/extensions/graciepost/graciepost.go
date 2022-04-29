package graciepost

import (
  "net/http"
  "log"
  "fmt"
  "encoding/json"
  "strings"

  "github.com/bwmarrin/discordgo"
  "github.com/gracieart/bubblebot"

  "github.com/gracieart/graciebot/src/extensions/like"
  "github.com/thoas/go-funk"
)


type PostMeta struct {
  Key           string
  Title         string
  Artist        string
  Pfp           string
  Desc          string
  PostLink      string
  ImageLink     string
  SiteName      string
  Channel       string
  OverrideEmbed bool
}



type GraciePost struct {
  extensionID string
  extensionInfo bubble.ExtensionInfo
  bot *bubble.Bot
  port string
  charLimit int
  ext_like *like.Like
  key string
}

func (g *GraciePost) ExtensionID() string { return g.extensionID }
func (g *GraciePost) ExtensionInfo() bubble.ExtensionInfo { return g.extensionInfo }


type Config struct {
  CharLimit int
  Port string
  Key string
}


func New(cnf Config) *GraciePost {
  gp := &GraciePost{
    extensionID : "graciebell.art.graciepost",
    port : "30034",
    charLimit : 180,
    key : cnf.Key,
    extensionInfo: bubble.ExtensionInfo{
      Name: "GraciePost",
      Description: "Post images from your browser using the GraciePost Firefox extension.",
    },
  }
  if cnf.Port != "" { gp.port = cnf.Port }
  if cnf.CharLimit > 0 { gp.charLimit = cnf.CharLimit }
  return gp
}


func (g *GraciePost) Load(b *bubble.Bot) error {
  g.bot = b

  if l, ok := g.bot.FindExtension("graciebell.art.like"); ok {
    g.ext_like = l.(*like.Like)
  }

  http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "*")

    switch r.Method {
    case "GET":
      w.Header().Set("Content-Type", "application/json")
      w.Write(g.GetChannels())


    case "POST":
      decoder := json.NewDecoder(r.Body)
      var meta PostMeta
      err := decoder.Decode(&meta)
      if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
      }

      if meta.Key != g.key {
        http.Error(w, "401 unauthorized.", http.StatusUnauthorized)
        return
      }
      g.Post(meta)


    default:
      http.Error(w, "501 method not implmemented.", http.StatusNotImplemented)
    }
  })

  return nil
}



func (g *GraciePost) OnLifecycleEvent(l bubble.LifecycleEvent) error {
  switch l {
  case bubble.LE_Connect:
    go g.listen()
  }
  return nil
}


func (g *GraciePost) listen() {
  log.Fatal(http.ListenAndServe(":" + g.port, nil))
}



func (g *GraciePost) Post(meta PostMeta) {
  msg, err := g.bot.Session.ChannelMessageSendComplex(meta.Channel, g.createMsg(meta))
  if err != nil { fmt.Printf("GraciePost: %s", err) }

  // add like button if the plugin is connected
  if g.ext_like != nil { g.ext_like.AddLike(msg) }
}


func (g *GraciePost) createMsg(meta PostMeta) *discordgo.MessageSend {
  // just return the link if its set to override embed
  if meta.OverrideEmbed {
    return &discordgo.MessageSend{ Content: meta.PostLink }
  }

  // Truncate the description (if any)
  if len(meta.Desc) > 0 {
    cutoff := 0
    // set cutoff point at the first double linebreak (if any)
    if i := strings.Index(meta.Desc, "\n\n"); i != -1 { cutoff = i }
    // set cutoff point to the CharLimit if it exceeds it
    cutoff = funk.MinInt( []int{ cutoff, g.charLimit } )
    // truncate the string if cutoff was set
    if cutoff > 0 { meta.Desc = meta.Desc[:cutoff] + " ..." }
  }

  // create & send the embed
  msg := &discordgo.MessageSend{
    Embeds: []*discordgo.MessageEmbed{
      &discordgo.MessageEmbed{
        Title: meta.Title,
        Author: &discordgo.MessageEmbedAuthor{
          Name: meta.Artist,
          IconURL: meta.Pfp,
        },
        Description: meta.Desc,
        Fields: []*discordgo.MessageEmbedField{
          &discordgo.MessageEmbedField{
            Name: "Post link:",
            Value: meta.PostLink,
          },
        },
        Image: &discordgo.MessageEmbedImage{
          URL: meta.ImageLink,
        },
        Footer: &discordgo.MessageEmbedFooter{
          Text: "Retrieved from " + meta.SiteName + " using GraciePost",
        },
      },
    },
  }

  //log.Print(msg.Embeds[0])
  return msg
}




type Menu struct {
  Title string `json:"title"`
  ID string `json:"id"`
  ParentID string `json:"parentId"`
}

type MenuLevel struct {
  Name string `json:"name"`
  Items []Menu `json:"items"`
}

// get the object with all the menus to send back to the GraciePost extension
func (g *GraciePost) GetChannels() []byte {
  // get guilds. (have to do it this way kuz the guilds in State.Ready
  // are not populated with the channels)
  tempGuilds := g.bot.Session.State.Ready.Guilds
  guilds := funk.Map(tempGuilds,
    func(tempGuild *discordgo.Guild) *discordgo.Guild {
      guild, _ := g.bot.Session.Guild(tempGuild.ID)
      return guild
    },
  ).([]*discordgo.Guild)

  // we create this map to filter out the empty categories later on
  categorySizes := make(map[string]int)

  // get all channels
  textChannels := []*discordgo.Channel{}
  categories := []*discordgo.Channel{}
  for _, guild := range guilds {
    guildChannels, _ := g.bot.Session.GuildChannels(guild.ID)

    // separate text and category channels
    for _, ch := range guildChannels {
      switch ch.Type {
      case discordgo.ChannelTypeGuildText:
        textChannels = append(textChannels, ch)
        if ch.ParentID != "" { categorySizes[ch.ParentID]++ }
      case discordgo.ChannelTypeGuildCategory:
        categories = append(categories, ch)
      }
    }
  }

  // filter out categories with no text channels
  categories = funk.Filter(categories, func(cat *discordgo.Channel) bool {
    if categorySizes[cat.ID] > 0 { return true } else { return false }
  }).([]*discordgo.Channel)

  // create the struct to be sent as json
  menus := []MenuLevel{
    MenuLevel{
      Name: "guilds",
      Items: funk.Map(guilds, func(guild *discordgo.Guild) Menu {
        return Menu{
          Title: guild.Name,
          ID: guild.ID,
        }
      }).([]Menu),
    },

    MenuLevel{
      Name: "categories",
      Items: funk.Map(categories, func(cat *discordgo.Channel) Menu {
        return Menu{
          Title: cat.Name,
          ID: cat.ID,
          ParentID: cat.GuildID,
        }
      }).([]Menu),
    },

    MenuLevel{
      Name: "channels",
      Items: funk.Map(textChannels, func(ch *discordgo.Channel) Menu {
        m := Menu{
          Title: ch.Name,
          ID: ch.ID,
          ParentID: ch.ParentID,
        }
        if m.ParentID == "" { m.ParentID = ch.GuildID }
        return m
      }).([]Menu),
    },
  }

  // marshal and return
  res, _ := json.Marshal(menus)
  return res
}
