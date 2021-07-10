package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import "sync"
import "sync/atomic"
import "../labrpc"
// import "runtime"
import "fmt"
import "time"
import "math/rand"


// func Filter(vs []*labrpc.ClientEnd, f func(int) bool) []*labrpc.ClientEnd {
// 	filtered := make([]*labrpc.ClientEnd, 0)
// 	for i, v := range vs {
// 		if f(i) {
// 			filtered = append(filtered, v)
// 		}
// 	}

// 	return filtered
// }


//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

type Log struct {
	term		int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	heartbeat	chan  bool
	identity	string
	timer		*time.Timer

	// persistent state across all servers (updated on stable storage before responding to RPCs)
	currentTerm		int // latest term server has seen
	votedFor		*int // candidateId that received vote in current term (or null if none)
	logs			map[int]Log // log entries, each one contains a command for the state machine, and a term when the entry was received by the leader

	// Volatile state on all servers
	commitIndex		int // index of highest log entry known to be committed (initialized to 0, increases monotonically)
	lastApplied 	int // index of highest log entry applied to state machine (initialized to 0, increases monotonically)

	// volatile state on leaders (reinitialized after election)
	nextIndex		map[int]int // for each server, index of the next log entry to send to that server
	matchIndex		map[int]int // for each server, index of highest log entry known to be replicated on server (initialized to 0, increases monotonically)
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	fmt.Printf("%v current term %v\n", rf.me, rf.currentTerm)
	term := rf.currentTerm
	isleader := (rf.identity == "leader")
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}


//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}




//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	Term			int		// candidates term
	CandidateId		int		// the candidate requesting the vote
	LastLogIndex	int		// index of candidate's last log entry
	LastLogTerm		int		// term of candidate's last log entry
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	Term				int		// current term, for candidate to update itself
	VoteGranted			bool	// true means candidate received vote
	HeartbeatResponse	bool	// true for a positive response (consensus with the caller)
}

type AppendEntriesArgs struct {
	Term			int // leaders term
	LeaderId		int	// so followers can redirect clients
	PrevLogIndex	int	// index of log entry immediately preceding new ones
	PrevLogTerm		int	// term of prevLogIndex entry
	Entries			map[int]interface{}	// log entries to store (empty for heartbeat; may send more than one for efficiency)
	LeaderCommit	int	// leader's commitIndex
	IsHeartbeat		bool
}

type AppendEntriesReply struct {
	Term				int		// current term, for a former leader to update itself
	HeartbeatResponse	bool	// true if follower contained an entry matching	prevLogEntry and prevLogTerm
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *RequestVoteReply) {
	// first, check if this call is a heartbeat or a real AppendEntries request
	if (args.IsHeartbeat) {
		// runtime.Breakpoint()
		reply.HeartbeatResponse = (*rf.votedFor == args.LeaderId)
		fmt.Printf("%v's leader is %v and the requesting heartbeat LeaderId is %v\n", rf.me, *rf.votedFor, args.LeaderId)

		if (reply.HeartbeatResponse) {
			// reset timer for election phase
			rf.heartbeat <- true
		}

		return
	}
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	isLaterTermThanThisFollower := false
	isThisFollowersVoteSpokenFor := true
	isCandidatesLogUpToDate := false
	if (args.Term > rf.currentTerm) {
		rf.currentTerm = args.Term
		isLaterTermThanThisFollower = true
	}

	// Each value of an empty struct variable is set to the zero value for its type, int is 0 (not nil)
	if (rf.votedFor == nil || *rf.votedFor == args.CandidateId) {
		isThisFollowersVoteSpokenFor = false
		// add a check that candidate’s log is at least as up-to-date as receiver’s log
		isCandidatesLogUpToDate = true
	}

	// runtime.Breakpoint()
	reply.VoteGranted = isLaterTermThanThisFollower && !isThisFollowersVoteSpokenFor && isCandidatesLogUpToDate
	if (reply.VoteGranted) {
		if (rf.votedFor == nil) {
			// this block will hit the first time the node votes for a leader
			// and is needed to support a nullable value for rf.votedFor
			rf.votedFor = new(int)
		}
		*rf.votedFor = args.CandidateId
		go rf.startLifecycle("follower")
		fmt.Printf("%v voted for %v\n", rf.me, *rf.votedFor)
	} else {
		reply.Term = rf.currentTerm
		fmt.Printf("%v declined to vote for %v\n", rf.me, args.CandidateId)
	}
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendHeartbeat(server int) bool {
	reply := AppendEntriesReply{}
	args := AppendEntriesArgs{
		Term: rf.currentTerm,
		LeaderId: *rf.votedFor,
		PrevLogIndex: 0,
		PrevLogTerm: 0,
		Entries: make(map[int]interface{}),
		LeaderCommit: 0,
		IsHeartbeat: true,
	}
	rf.peers[server].Call("Raft.AppendEntries", &args, &reply)

	return reply.HeartbeatResponse
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}


//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).


	return index, term, isLeader
}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) startLeaderElection() {
	fmt.Printf("starting leader election %v\n", rf.me)

	rf.startLifecycle("candidate")
}

func (rf *Raft) startHeartbeatTimer() {
	waitTime := rf.randomTimeGenerator()
	// rf.timer = time.AfterFunc(waitTime * time.Millisecond, rf.startLeaderElection)

	select {
	case <-rf.heartbeat:
		fmt.Printf("%v starting heartbeat over\n", rf.me)

		go rf.startHeartbeatTimer()
	case <-time.After(waitTime):
		if (rf.identity != "leader") {
			fmt.Printf("%v timed out in term %v after %v\n", rf.me, rf.currentTerm, waitTime)
			go rf.startLeaderElection()
		}
	}
}

// func (rf *Raft) heartbeat(server int) {
// 	rf.sendHeartbeat(server)
// 	// runtime.Breakpoint()

// 	// Stop prevents the Timer from firing. It returns true if the call stops the timer, false if the timer has
// 	// already expired or been stopped. Stop does not close the channel, to prevent a read from the channel succeeding incorrectly.
// 	if (!rf.timer.Stop()) {
// 		fmt.Printf("request from %v to %v timed out ", rf.me, server)
// 		// To ensure the channel is empty after a call to Stop, check the return value and drain the channel.
// 		<- rf.timer.C

// 		rf.startLifecycle("candidate")
// 	}
// }

func (rf *Raft) updateFollowers(server int) {
	if (server == rf.me) {
		return
	}
	fmt.Printf("%v sent a heartbeat to %v\n", rf.me, server)
	ok := rf.sendHeartbeat(server)

	if (ok) {
		fmt.Printf("%v server responded ok to %v\n", server, rf.me)
	} else {
		fmt.Printf("%v server responded not ok to %v\n", server, rf.me)
	}
}

func (rf *Raft) startLifecycle(lifecycle string) {

	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	rf.identity = lifecycle

	if (lifecycle == "follower") {
		rf.heartbeat <- true
	} else if (lifecycle == "candidate") {
		// we're here because of a timeout, so restart the heartbeat timer
		go rf.startHeartbeatTimer()

		if (rf.votedFor == nil) {
			// this block will hit the first time the node votes for a leader
			// and is needed to support a nullable value for rf.votedFor
			rf.votedFor = new(int)
		}
		*rf.votedFor = rf.me
		rf.currentTerm = rf.currentTerm + 1

		votes := make([]int, len(rf.peers))
		votes[rf.me] = 1 // current node votes for itself

		var wg sync.WaitGroup

		for i := range rf.peers {
			logLength := len(rf.logs)
			lastLogIndex := 0
			lastLogTerm := 0
			if (logLength > 0) {
				lastLogIndex = logLength + 1 // logs are 1 indexed according to the spec
				lastLogTerm = rf.logs[logLength - 1].term
			}

			if (i == rf.me) {
				votes[i] = 1
			} else {
				args := RequestVoteArgs{Term: rf.currentTerm, CandidateId: rf.me, LastLogIndex: lastLogIndex, LastLogTerm: lastLogTerm }
				reply := RequestVoteReply{}

				rf.sendRequestVote(i, &args, &reply)

				if (reply.VoteGranted) {
					votes[i] = 1
					} else {
						if (reply.Term > rf.currentTerm) {
							rf.currentTerm = reply.Term
						}
					}
			}

			// we run the next codeblock synchronously across the parent loop's iterations
			// to avoid responses arriving at the same time and starting 2 leader lifecycles
			wg.Wait()
			wg.Add(1)
			// if the leader phase has already started, this sendRequestVote resolved after the candidate already got
			// enough votes to become the leader
			if (rf.identity != "leader") {
				voteTotal := 0
				for _, vote := range votes {
					voteTotal += vote
					if (voteTotal > (len(votes) / 2)) {
						fmt.Printf("%v starting leader phase with votes %v\n", rf.me, votes)
						rf.identity = "leader" // need to update identity early so waitGroup behaves correctly
						go rf.startLifecycle("leader")
						break
					}
				}
			}
			wg.Done()
		}
	} else if (lifecycle == "leader") {
		for i := range rf.peers {
			go rf.updateFollowers(i)
		}

		waitTime := rf.randomTimeGenerator() / 3
		fmt.Printf("%v waiting %v\n", rf.me, waitTime)
		time.Sleep(waitTime)

		go rf.startLifecycle("leader")
	}
}

func (rf *Raft) randomTimeGenerator() time.Duration {
	time.Sleep(time.Duration(rf.me) * time.Nanosecond) // this is to prevent nodes from having the same seeded value for rand
	rand.Seed(time.Now().UnixNano())
	min := int(150 * 1000000)
	max := int(300 * 1000000)
	return time.Duration(rand.Intn(max - min + 1) + min)
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.currentTerm = 0
	rf.heartbeat = make(chan bool)

	go rf.startHeartbeatTimer()
	go rf.startLifecycle("follower")

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())


	return rf
}
